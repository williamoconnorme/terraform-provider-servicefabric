package provider

import (
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
