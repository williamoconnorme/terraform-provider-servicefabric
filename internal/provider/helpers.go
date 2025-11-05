package provider

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func convertStringMapToAttrValues(input map[string]string) map[string]attr.Value {
	if len(input) == 0 {
		return map[string]attr.Value{}
	}
	output := make(map[string]attr.Value, len(input))
	for k, v := range input {
		output[k] = types.StringValue(v)
	}
	return output
}

func stringMapEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func applicationCompositeID(typeName, name string) string {
	if typeName == "" {
		return name
	}
	return typeName + "|" + name
}

func splitApplicationCompositeID(id string) (string, string, bool) {
	parts := strings.SplitN(id, "|", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
