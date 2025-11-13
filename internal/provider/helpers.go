package provider

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/williamoconnorme/terraform-provider-servicefabric/internal/servicefabric"
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

func serviceKindFromInfo(info servicefabric.ServiceInfo) string {
	if info.ServiceKind != "" {
		return info.ServiceKind
	}
	return info.Kind
}

func deriveApplicationNameFromService(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("service name required")
	}
	trimmed := strings.TrimSuffix(name, "/")
	lastSlash := strings.LastIndex(trimmed, "/")
	if lastSlash == -1 {
		return "", fmt.Errorf("service name %q does not include an application path", name)
	}
	if lastSlash == len("fabric:")-1 {
		return "", fmt.Errorf("service name %q is missing an application segment", name)
	}
	return trimmed[:lastSlash], nil
}

func stringValue(v types.String) (string, bool) {
	if v.IsNull() || v.IsUnknown() {
		return "", false
	}
	return v.ValueString(), true
}

func int64Value(v types.Int64) (int64, bool) {
	if v.IsNull() || v.IsUnknown() {
		return 0, false
	}
	return v.ValueInt64(), true
}

func boolValue(v types.Bool) (bool, bool) {
	if v.IsNull() || v.IsUnknown() {
		return false, false
	}
	return v.ValueBool(), true
}
