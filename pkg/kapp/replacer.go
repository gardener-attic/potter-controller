package kapp

const (
	secretRef     = "secretRef"
	secretRefName = "name"
)

// Recursively traverses the tree structure of obj. For every key-value pair, the value is replaced according to the
// mapping, provided that:
// 1. key = "name" and parent key = "secretRef",
// 2. the value has type string, and
// 3. there is a mapping for the value.
func (r *kappDeployerDI) replace(obj interface{}, mapping map[string]string, parentKey string) {
	switch typedObj := obj.(type) {
	case []interface{}:
		for i := range typedObj {
			r.replace(typedObj[i], mapping, "")
		}

	case map[string]interface{}:
		for key, value := range typedObj {
			switch typedValue := value.(type) {
			case string:
				if key == secretRefName && parentKey == secretRef {
					if mappedValue, ok := mapping[typedValue]; ok {
						typedObj[key] = mappedValue
					}
				}

			case []interface{}:
				r.replace(typedValue, mapping, "")

			case map[string]interface{}:
				r.replace(typedValue, mapping, key)
			}
		}
	}
}
