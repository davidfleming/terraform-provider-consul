package consul

import (
	"strings"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

func resourceConsulPreparedQuery() *schema.Resource {
	return &schema.Resource{
		Create: resourceConsulPreparedQueryCreate,
		Update: resourceConsulPreparedQueryUpdate,
		Read:   resourceConsulPreparedQueryRead,
		Delete: resourceConsulPreparedQueryDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		SchemaVersion: 0,

		Schema: map[string]*schema.Schema{
			"datacenter": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"name": {
				Type:     schema.TypeString,
				Required: true,
			},

			"session": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"token": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
			},

			"stored_token": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"service": {
				Type:     schema.TypeString,
				Required: true,
			},

			"tags": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},

			"near": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"only_passing": {
				Type:     schema.TypeBool,
				Optional: true,
			},

			"connect": {
				Type:     schema.TypeBool,
				Optional: true,
			},

			"ignore_check_ids": {
				Type:     schema.TypeList,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},

			"node_meta": {
				Type:     schema.TypeMap,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},

			"service_meta": {
				Type:     schema.TypeMap,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},

			"failover": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"nearest_n": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"datacenters": {
							Type:     schema.TypeList,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
					},
				},
			},

			"dns": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"ttl": {
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
			},

			"template": {
				Type:     schema.TypeList,
				MaxItems: 1,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"type": {
							Type:     schema.TypeString,
							Required: true,
						},
						"regexp": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
		},
	}
}

func resourceConsulPreparedQueryCreate(d *schema.ResourceData, meta interface{}) error {
	client, _, wOpts := getClient(d, meta)
	pq := preparedQueryDefinitionFromResourceData(d)

	id, _, err := client.PreparedQuery().Create(pq, wOpts)
	if err != nil {
		return err
	}

	d.SetId(id)
	return resourceConsulPreparedQueryRead(d, meta)
}

func resourceConsulPreparedQueryUpdate(d *schema.ResourceData, meta interface{}) error {
	client, _, wOpts := getClient(d, meta)
	pq := preparedQueryDefinitionFromResourceData(d)

	if _, err := client.PreparedQuery().Update(pq, wOpts); err != nil {
		return err
	}

	return resourceConsulPreparedQueryRead(d, meta)
}

func resourceConsulPreparedQueryRead(d *schema.ResourceData, meta interface{}) error {
	client, qOpts, _ := getClient(d, meta)

	queries, _, err := client.PreparedQuery().Get(d.Id(), qOpts)
	if err != nil {
		// Check for a 404/not found, these are returned as errors.
		if strings.Contains(err.Error(), "not found") {
			d.SetId("")
			return nil
		}
		return err
	}

	if len(queries) != 1 {
		d.SetId("")
		return nil
	}
	pq := queries[0]

	sw := newStateWriter(d)
	sw.set("name", pq.Name)
	sw.set("session", pq.Session)
	sw.set("stored_token", pq.Token)
	sw.set("service", pq.Service.Service)
	sw.set("near", pq.Service.Near)
	sw.set("only_passing", pq.Service.OnlyPassing)
	sw.set("connect", pq.Service.Connect)
	sw.set("tags", pq.Service.Tags)
	sw.set("ignore_check_ids", pq.Service.IgnoreCheckIDs)
	sw.set("node_meta", pq.Service.NodeMeta)
	sw.set("service_meta", pq.Service.ServiceMeta)

	// Since failover and dns are implemented with an optionnal list instead of a
	// sub-resource, writing those attributes to the state is more involved that
	// it needs to.

	failover := make([]map[string]interface{}, 0)

	// First we must find whether the user wrote a failover block
	userWroteFailover := len(d.Get("failover").([]interface{})) != 0

	// We must write a failover block if the user wrote one or if one of the values
	// differ from the defaults
	if userWroteFailover || pq.Service.Failover.NearestN > 0 || len(pq.Service.Failover.Datacenters) > 0 {
		failover = append(failover, map[string]interface{}{
			"nearest_n":   pq.Service.Failover.NearestN,
			"datacenters": pq.Service.Failover.Datacenters,
		})
	}

	// We can finally set the failover attribute
	sw.set("failover", failover)

	dns := make([]map[string]interface{}, 0)

	userWroteDNS := len(d.Get("dns").([]interface{})) != 0

	if userWroteDNS || pq.DNS.TTL != "" {
		dns = append(dns, map[string]interface{}{
			"ttl": pq.DNS.TTL,
		})
	}
	sw.set("dns", dns)

	template := make([]map[string]interface{}, 0)

	userWroteTemplate := len(d.Get("template").([]interface{})) != 0

	if userWroteTemplate || pq.Template.Type != "" {
		template = append(template, map[string]interface{}{
			"type":   pq.Template.Type,
			"regexp": pq.Template.Regexp,
		})
	}
	sw.set("template", template)

	return sw.error()
}

func resourceConsulPreparedQueryDelete(d *schema.ResourceData, meta interface{}) error {
	client, _, wOpts := getClient(d, meta)

	if _, err := client.PreparedQuery().Delete(d.Id(), wOpts); err != nil {
		return err
	}

	d.SetId("")
	return nil
}

func preparedQueryDefinitionFromResourceData(d *schema.ResourceData) *consulapi.PreparedQueryDefinition {
	pq := &consulapi.PreparedQueryDefinition{
		ID:      d.Id(),
		Name:    d.Get("name").(string),
		Session: d.Get("session").(string),
		Token:   d.Get("stored_token").(string),
		Service: consulapi.ServiceQuery{
			Service:     d.Get("service").(string),
			Near:        d.Get("near").(string),
			OnlyPassing: d.Get("only_passing").(bool),
			Connect:     d.Get("connect").(bool),
		},
	}

	tags := d.Get("tags").(*schema.Set).List()
	pq.Service.Tags = make([]string, len(tags))
	for i, v := range tags {
		pq.Service.Tags[i] = v.(string)
	}

	pq.Service.NodeMeta = make(map[string]string)
	for k, v := range d.Get("node_meta").(map[string]interface{}) {
		pq.Service.NodeMeta[k] = v.(string)
	}

	pq.Service.ServiceMeta = make(map[string]string)
	for k, v := range d.Get("service_meta").(map[string]interface{}) {
		pq.Service.ServiceMeta[k] = v.(string)
	}

	ignoreCheckIDs := d.Get("ignore_check_ids").([]interface{})
	pq.Service.IgnoreCheckIDs = make([]string, len(ignoreCheckIDs))
	for i, id := range ignoreCheckIDs {
		pq.Service.IgnoreCheckIDs[i] = id.(string)
	}

	if _, ok := d.GetOk("failover.0"); ok {
		failover := consulapi.QueryFailoverOptions{
			NearestN: d.Get("failover.0.nearest_n").(int),
		}

		dcs := d.Get("failover.0.datacenters").([]interface{})
		failover.Datacenters = make([]string, len(dcs))
		for i, v := range dcs {
			failover.Datacenters[i] = v.(string)
		}

		pq.Service.Failover = failover
	}

	if _, ok := d.GetOk("template.0"); ok {
		pq.Template = consulapi.QueryTemplate{
			Type:   d.Get("template.0.type").(string),
			Regexp: d.Get("template.0.regexp").(string),
		}
	}

	if _, ok := d.GetOk("dns.0"); ok {
		pq.DNS = consulapi.QueryDNSOptions{
			TTL: d.Get("dns.0.ttl").(string),
		}
	}

	return pq
}
