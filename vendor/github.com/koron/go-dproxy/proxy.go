package dproxy

// Proxy is a proxy to access a document (interface{}).
type Proxy interface {
	// Nil returns true, if target value is nil.
	Nil() bool

	// Value returns a proxied value.  If there are no values, it returns
	// error.
	Value() (interface{}, error)

	// Bool returns its value.  If value isn't the type, it returns error.
	Bool() (bool, error)

	// Int64 returns its value.  If value isn't the type, it returns error.
	Int64() (int64, error)

	// Float64 returns its value.  If value isn't the type, it returns error.
	Float64() (float64, error)

	// String returns its value.  If value isn't the type, it returns error.
	String() (string, error)

	// Array returns its value.  If value isn't the type, it returns error.
	Array() ([]interface{}, error)

	// Map returns its value.  If value isn't the type, it returns error.
	Map() (map[string]interface{}, error)

	// A returns an item from value treated as the array.
	A(n int) Proxy

	// M returns an item from value treated as the map.
	M(k string) Proxy

	// P returns which pointed by JSON Pointer's query q.
	P(q string) Proxy

	// ProxySet returns a set which converted from its array value.
	ProxySet() ProxySet

	// Q returns set of all items which property matchs with k.
	Q(k string) ProxySet

	// findJPT returns a match of JSON Pointer's Token t.
	findJPT(t string) Proxy

	// Proxy implements frame.
	frame
}

// ProxySet proxies to access to set.
type ProxySet interface {
	// Empty returns true when the set is empty.
	Empty() bool

	// Len returns count of items in the set.
	Len() int

	// BoolArray returns []bool which converterd from the set.
	BoolArray() ([]bool, error)

	// Int64Array returns []int64 which converterd from the set.
	Int64Array() ([]int64, error)

	// Float64Array returns []float64 which converterd from the set.
	Float64Array() ([]float64, error)

	// StringArray returns []string which converterd from the set.
	StringArray() ([]string, error)

	// ArrayArray returns [][]interface{} which converterd from the set.
	ArrayArray() ([][]interface{}, error)

	// MapArray returns []map[string]interface{} which converterd from the set.
	MapArray() ([]map[string]interface{}, error)

	// ProxyArray returns []Proxy which wrap each items.
	ProxyArray() ([]Proxy, error)

	// A returns an proxy for index in the set.
	A(n int) Proxy

	// Q returns set of all items which property matchs with k.
	Q(k string) ProxySet

	// Qc returns set of property of all items.
	Qc(k string) ProxySet

	// Proxy implements frame.
	frame
}

// New creates a new Proxy instance for v.
func New(v interface{}) Proxy {
	return &valueProxy{value: v}
}

// NewSet create a new ProxySet instance for v.
func NewSet(v []interface{}) ProxySet {
	return &setProxy{values: v}
}
