package dproxy

// Type is type of value.
type Type int

const (
	// Tunknown shows value is not supported.
	Tunknown Type = iota

	// Tnil shows value is nil.
	Tnil

	// Tbool shows value is bool.
	Tbool

	// Tint64 shows value is int64.
	Tint64

	// Tfloat64 shows value is float64.
	Tfloat64

	// Tstring shows value is string.
	Tstring

	// Tarray shows value is an array ([]interface{})
	Tarray

	// Tmap shows value is a map (map[string]interface{})
	Tmap
)

// detectType returns type of a value.
func detectType(v interface{}) Type {
	if v == nil {
		return Tnil
	}
	switch v.(type) {
	case bool:
		return Tbool
	case int, int32, int64:
		return Tint64
	case float32, float64:
		return Tfloat64
	case string:
		return Tstring
	case []interface{}:
		return Tarray
	case map[string]interface{}:
		return Tmap
	default:
		return Tunknown
	}
}

func (t Type) String() string {
	switch t {
	case Tunknown:
		return "unknown"
	case Tnil:
		return "nil"
	case Tbool:
		return "bool"
	case Tint64:
		return "int64"
	case Tfloat64:
		return "float64"
	case Tstring:
		return "string"
	case Tarray:
		return "array"
	case Tmap:
		return "map"
	default:
		return "unknown"
	}
}
