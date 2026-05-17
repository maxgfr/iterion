package runtime

import (
	"testing"
)

// TestCopyOutputs_DeepCopiesNestedMaps guards the data-integrity fix
// that promoted copyOutputs from a two-level shallow clone to a full
// recursive copy. Two parallel branches sharing a nested
// map[string]interface{} from an upstream node previously raced on
// the inner hashtable; the deep copy gives each branch its own
// independent tree.
func TestCopyOutputs_DeepCopiesNestedMaps(t *testing.T) {
	src := map[string]map[string]interface{}{
		"node_a": {
			"obj": map[string]interface{}{
				"k1": "v1",
				"k2": map[string]interface{}{"nested": "value"},
			},
			"list": []interface{}{
				"a",
				map[string]interface{}{"in_list": true},
			},
		},
	}

	dst := copyOutputs(src)

	// Mutate the destination's deep structure and ensure the source
	// is untouched.
	dstObj := dst["node_a"]["obj"].(map[string]interface{})
	dstObj["k1"] = "mutated"
	dstObj["k2"].(map[string]interface{})["nested"] = "mutated"
	dst["node_a"]["list"].([]interface{})[1].(map[string]interface{})["in_list"] = false

	srcObj := src["node_a"]["obj"].(map[string]interface{})
	if srcObj["k1"] != "v1" {
		t.Errorf("source map mutated via shallow copy: k1=%v", srcObj["k1"])
	}
	if nested := srcObj["k2"].(map[string]interface{})["nested"]; nested != "value" {
		t.Errorf("source nested map mutated via shallow copy: nested=%v", nested)
	}
	if inList := src["node_a"]["list"].([]interface{})[1].(map[string]interface{})["in_list"]; inList != true {
		t.Errorf("source slice element mutated via shallow copy: in_list=%v", inList)
	}
}

// TestCopyOutputs_PreservesScalars is the inverse — scalars are
// safe to share by value, so the deep copy still returns them as-is.
func TestCopyOutputs_PreservesScalars(t *testing.T) {
	src := map[string]map[string]interface{}{
		"node_b": {
			"s":  "string",
			"i":  42,
			"f":  3.14,
			"b":  true,
			"n":  nil,
			"by": []byte("bytes"),
		},
	}
	dst := copyOutputs(src)
	if got := dst["node_b"]["s"]; got != "string" {
		t.Errorf("scalar string copy: %v", got)
	}
	if got := dst["node_b"]["i"]; got != 42 {
		t.Errorf("scalar int copy: %v", got)
	}
}
