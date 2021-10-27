package dproxy

import "bytes"

// Drain stores errors from Proxy or ProxySet.
type Drain struct {
	errors []error
}

// Has returns true if the drain stored some of errors.
func (d *Drain) Has() bool {
	return d != nil && len(d.errors) > 0
}

// First returns a stored error.  Returns nil if there are no errors.
func (d *Drain) First() error {
	if d == nil || len(d.errors) == 0 {
		return nil
	}
	return d.errors[0]
}

// All returns all errors which stored.  Return nil if no errors stored.
func (d *Drain) All() []error {
	if d == nil || len(d.errors) == 0 {
		return nil
	}
	a := make([]error, 0, len(d.errors))
	return append(a, d.errors...)
}

// CombineErrors returns an error which combined all stored errors.  Return nil
// if not erros stored.
func (d *Drain) CombineErrors() error {
	if d == nil || len(d.errors) == 0 {
		return nil
	}
	return drainError(d.errors)
}

func (d *Drain) put(err error) {
	if err == nil {
		return
	}
	d.errors = append(d.errors, err)
}

// Bool returns bool value and stores an error.
func (d *Drain) Bool(p Proxy) bool {
	v, err := p.Bool()
	d.put(err)
	return v
}

// Int64 returns int64 value and stores an error.
func (d *Drain) Int64(p Proxy) int64 {
	v, err := p.Int64()
	d.put(err)
	return v
}

// Float64 returns float64 value and stores an error.
func (d *Drain) Float64(p Proxy) float64 {
	v, err := p.Float64()
	d.put(err)
	return v
}

// String returns string value and stores an error.
func (d *Drain) String(p Proxy) string {
	v, err := p.String()
	d.put(err)
	return v
}

// Array returns []interface{} value and stores an error.
func (d *Drain) Array(p Proxy) []interface{} {
	v, err := p.Array()
	d.put(err)
	return v
}

// Map returns map[string]interface{} value and stores an error.
func (d *Drain) Map(p Proxy) map[string]interface{} {
	v, err := p.Map()
	d.put(err)
	return v
}

// BoolArray returns []bool value and stores an error.
func (d *Drain) BoolArray(ps ProxySet) []bool {
	v, err := ps.BoolArray()
	d.put(err)
	return v
}

// Int64Array returns []int64 value and stores an error.
func (d *Drain) Int64Array(ps ProxySet) []int64 {
	v, err := ps.Int64Array()
	d.put(err)
	return v
}

// Float64Array returns []float64 value and stores an error.
func (d *Drain) Float64Array(ps ProxySet) []float64 {
	v, err := ps.Float64Array()
	d.put(err)
	return v
}

// StringArray returns []string value and stores an error.
func (d *Drain) StringArray(ps ProxySet) []string {
	v, err := ps.StringArray()
	d.put(err)
	return v
}

// ArrayArray returns [][]interface{} value and stores an error.
func (d *Drain) ArrayArray(ps ProxySet) [][]interface{} {
	v, err := ps.ArrayArray()
	d.put(err)
	return v
}

// MapArray returns []map[string]interface{} value and stores an error.
func (d *Drain) MapArray(ps ProxySet) []map[string]interface{} {
	v, err := ps.MapArray()
	d.put(err)
	return v
}

// ProxyArray returns []Proxy value and stores an error.
func (d *Drain) ProxyArray(ps ProxySet) []Proxy {
	v, err := ps.ProxyArray()
	d.put(err)
	return v
}

type drainError []error

func (derr drainError) Error() string {
	b := bytes.Buffer{}
	for i, err := range derr {
		if i > 0 {
			_, _ = b.WriteString("; ")
		}
		_, _ = b.WriteString(err.Error())
	}
	return b.String()
}
