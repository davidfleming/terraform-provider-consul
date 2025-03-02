package consul

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

func resourceConsulKeyPrefix() *schema.Resource {
	return &schema.Resource{
		Create: resourceConsulKeyPrefixCreate,
		Update: resourceConsulKeyPrefixUpdate,
		Read:   resourceConsulKeyPrefixRead,
		Delete: resourceConsulKeyPrefixDelete,
		Importer: &schema.ResourceImporter{
			State: func(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
				if err := d.Set("path_prefix", d.Id()); err != nil {
					return nil, fmt.Errorf("failed to set 'path_prefix': %v", err)
				}
				return []*schema.ResourceData{d}, nil
			},
		},

		Schema: map[string]*schema.Schema{
			"datacenter": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"token": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
			},

			"path_prefix": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"subkeys": {
				Type:     schema.TypeMap,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},

			"subkey": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"path": {
							Type:     schema.TypeString,
							Required: true,
						},

						"value": {
							Type:     schema.TypeString,
							Required: true,
						},

						"flags": {
							Type:     schema.TypeInt,
							Optional: true,
							Default:  0,
						},
					},
				},
			},

			"namespace": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"partition": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
		},
	}
}

func resourceConsulKeyPrefixCreate(d *schema.ResourceData, meta interface{}) error {
	keyClient := newKeyClient(d, meta)

	type subKey struct {
		value string
		flags int
	}

	pathPrefix := d.Get("path_prefix").(string)
	subKeys := map[string]subKey{}
	for k, vI := range d.Get("subkeys").(map[string]interface{}) {
		subKeys[k] = subKey{value: vI.(string), flags: 0}
	}

	// Add independant `subkey` attritbutes
	if subkeys, ok := d.GetOk("subkey"); ok {
		subkeysList := subkeys.(*schema.Set).List()
		for _, rawSubkey := range subkeysList {
			subkeyData := rawSubkey.(map[string]interface{})
			name := subkeyData["path"].(string)
			value := subkeyData["value"].(string)
			flags := subkeyData["flags"].(int)

			subKeys[name] = subKey{
				value: value,
				flags: flags,
			}
		}
	}

	// To reduce the impact of mistakes, we will only "create" a prefix that
	// is currently empty. This way we are less likely to accidentally
	// conflict with other mechanisms managing the same prefix.
	currentKVPairs, err := keyClient.GetUnderPrefix(pathPrefix)
	if err != nil {
		return err
	}
	if len(currentKVPairs) > 0 {
		return fmt.Errorf(
			"%d keys already exist under %s; delete them before managing this prefix with Terraform",
			len(currentKVPairs), pathPrefix,
		)
	}

	// Ideally we'd use d.Partial(true) here so we can correctly record
	// a partial write, but that mechanism doesn't work for individual map
	// members, so we record that the resource was created before we
	// do anything and that way we can recover from errors by doing an
	// Update on subsequent runs, rather than re-attempting Create with
	// some keys possibly already present.
	if pathPrefix == "" {
		d.SetId("/")
	} else {
		d.SetId(pathPrefix)
	}

	// Store the datacenter on this resource, which can be helpful for reference
	// in case it was read from the provider
	d.Set("datacenter", keyClient.qOpts.Datacenter)

	// Now we can just write in all the initial values, since we can expect
	// that nothing should need deleting yet, as long as there isn't some
	// other program racing us to write values... which we'll catch on a
	// subsequent Read.
	for name, subkey := range subKeys {
		fullPath := pathPrefix + name
		err := keyClient.Put(fullPath, subkey.value, subkey.flags)
		if err != nil {
			return fmt.Errorf("error while writing %s: %s", fullPath, err)
		}
	}

	return nil
}

func resourceConsulKeyPrefixUpdate(d *schema.ResourceData, meta interface{}) error {
	keyClient := newKeyClient(d, meta)

	pathPrefix := d.Get("path_prefix").(string)

	if d.HasChange("subkeys") {
		o, n := d.GetChange("subkeys")
		if o == nil {
			o = map[string]interface{}{}
		}
		if n == nil {
			n = map[string]interface{}{}
		}

		om := o.(map[string]interface{})
		nm := n.(map[string]interface{})

		// First we'll write all of the stuff in the "new map" nm,
		// and then we'll delete any keys that appear in the "old map" om
		// and do not also appear in nm. This ordering means that if a subkey
		// name is changed we will briefly have both the old and new names in
		// Consul, as opposed to briefly having neither.

		// Again, we'd ideally use d.Partial(true) here but it doesn't work
		// for maps and so we'll just rely on a subsequent Read to tidy up
		// after a partial write.

		// Write new and changed keys
		for k, vI := range nm {
			v := vI.(string)
			fullPath := pathPrefix + k
			err := keyClient.Put(fullPath, v, 0)
			if err != nil {
				return fmt.Errorf("error while writing %s: %s", fullPath, err)
			}
		}

		// Remove deleted keys
		for k := range om {
			if _, exists := nm[k]; exists {
				continue
			}
			fullPath := pathPrefix + k
			err := keyClient.Delete(fullPath)
			if err != nil {
				return fmt.Errorf("error while deleting %s: %s", fullPath, err)
			}
		}
	}

	// Update and remove keys from `subkey` attribute
	if d.HasChange("subkey") {
		oldKeys, newKeys := d.GetChange("subkey")
		if oldKeys == nil {
			oldKeys = &schema.Set{}
		}
		if newKeys == nil {
			newKeys = &schema.Set{}
		}

		// Create a map with old paths (no values)
		// We'll use it to determine which keys need to be deleted.
		// (the ones which are not in the new list)
		oldSubKeys := map[string]struct{}{}
		for _, rawKey := range oldKeys.(*schema.Set).List() {
			key := rawKey.(map[string]interface{})
			oldSubKeys[key["path"].(string)] = struct{}{}
		}

		// Upsert the new keys
		for _, rawSubkey := range newKeys.(*schema.Set).List() {
			key := rawSubkey.(map[string]interface{})

			name := key["path"].(string)
			value := key["value"].(string)
			flags := key["flags"].(int)

			// Delete from old keys (if exists) so it will not be removed in last step
			delete(oldSubKeys, name)

			fullPath := pathPrefix + name
			err := keyClient.Put(fullPath, value, flags)
			if err != nil {
				return fmt.Errorf("error while writing %s: %s", fullPath, err)
			}
		}

		// Remove remaining old subkey
		for path := range oldSubKeys {
			fullPath := pathPrefix + path
			err := keyClient.Delete(fullPath)
			if err != nil {
				return fmt.Errorf("error while deleting %s: %s", fullPath, err)
			}
		}
	}

	// Store the datacenter on this resource, which can be helpful for reference
	// in case it was read from the provider
	d.Set("datacenter", keyClient.qOpts.Datacenter)

	return nil
}

func resourceConsulKeyPrefixRead(d *schema.ResourceData, meta interface{}) error {
	keyClient := newKeyClient(d, meta)

	pathPrefix := d.Get("path_prefix").(string)

	pairs, err := keyClient.GetUnderPrefix(pathPrefix)
	if err != nil {
		return err
	}

	subKeys := make(map[string]string)
	subKeySet := make([]interface{}, 0)

	// We need to split subkeys fetched between the subkey and subkeys attributes:
	//   - everything whose path matches a given subkey in subkeyList goes in subkeySet
	//   - everything else goes into the subkeys attribute
	subkeyList := d.Get("subkey").(*schema.Set).List()
	for _, pair := range pairs {
		name := pair.Key[len(pathPrefix):]
		value := string(pair.Value)
		flags := int(pair.Flags)
		isSubkey := false

		for _, rawSubkey := range subkeyList {
			subkeyData := rawSubkey.(map[string]interface{})
			if name == subkeyData["path"] {
				isSubkey = true
				subkey := map[string]interface{}{
					"path":  name,
					"value": value,
					"flags": flags,
				}
				subKeySet = append(subKeySet, subkey)
				break
			}
		}

		if !isSubkey {
			subKeys[name] = string(value)
		}
	}

	sw := newStateWriter(d)

	sw.set("subkey", subKeySet)
	sw.set("subkeys", subKeys)

	// Store the datacenter on this resource, which can be helpful for reference
	// in case it was read from the provider
	sw.set("datacenter", keyClient.qOpts.Datacenter)

	return sw.error()
}

func resourceConsulKeyPrefixDelete(d *schema.ResourceData, meta interface{}) error {
	keyClient := newKeyClient(d, meta)

	pathPrefix := d.Get("path_prefix").(string)

	// Delete everything under our prefix, since the entire set of keys under
	// the given prefix is considered to be managed exclusively by Terraform.
	err := keyClient.DeleteUnderPrefix(pathPrefix)
	if err != nil {
		return err
	}

	d.SetId("")

	return nil
}
