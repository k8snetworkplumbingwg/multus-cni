package dproxy

import "strconv"

type setProxy struct {
	values []interface{}
	parent frame
	label  string
}

// setProxy implements ProxySet
var _ ProxySet = (*setProxy)(nil)

func (p *setProxy) Empty() bool {
	return p.Len() == 0
}

func (p *setProxy) Len() int {
	return len(p.values)
}

func (p *setProxy) BoolArray() ([]bool, error) {
	r := make([]bool, len(p.values))
	for i, v := range p.values {
		switch v := v.(type) {
		case bool:
			r[i] = v
		default:
			return nil, elementTypeError(p, i, Tbool, v)
		}
	}
	return r, nil
}

func (p *setProxy) Int64Array() ([]int64, error) {
	r := make([]int64, len(p.values))
	for i, v := range p.values {
		switch v := v.(type) {
		case int:
			r[i] = int64(v)
		case int32:
			r[i] = int64(v)
		case int64:
			r[i] = v
		case float32:
			r[i] = int64(v)
		case float64:
			r[i] = int64(v)
		default:
			return nil, elementTypeError(p, i, Tint64, v)
		}
	}
	return r, nil
}

func (p *setProxy) Float64Array() ([]float64, error) {
	r := make([]float64, len(p.values))
	for i, v := range p.values {
		switch v := v.(type) {
		case int:
			r[i] = float64(v)
		case int32:
			r[i] = float64(v)
		case int64:
			r[i] = float64(v)
		case float32:
			r[i] = float64(v)
		case float64:
			r[i] = v
		default:
			return nil, elementTypeError(p, i, Tfloat64, v)
		}
	}
	return r, nil
}

func (p *setProxy) StringArray() ([]string, error) {
	r := make([]string, len(p.values))
	for i, v := range p.values {
		switch v := v.(type) {
		case string:
			r[i] = v
		default:
			return nil, elementTypeError(p, i, Tstring, v)
		}
	}
	return r, nil
}

func (p *setProxy) ArrayArray() ([][]interface{}, error) {
	r := make([][]interface{}, len(p.values))
	for i, v := range p.values {
		switch v := v.(type) {
		case []interface{}:
			r[i] = v
		default:
			return nil, elementTypeError(p, i, Tarray, v)
		}
	}
	return r, nil
}

func (p *setProxy) MapArray() ([]map[string]interface{}, error) {
	r := make([]map[string]interface{}, len(p.values))
	for i, v := range p.values {
		switch v := v.(type) {
		case map[string]interface{}:
			r[i] = v
		default:
			return nil, elementTypeError(p, i, Tmap, v)
		}
	}
	return r, nil
}

func (p *setProxy) ProxyArray() ([]Proxy, error) {
	r := make([]Proxy, 0, len(p.values))
	for i, v := range p.values {
		r = append(r, &valueProxy{
			value:  v,
			parent: p,
			label:  "[" + strconv.Itoa(i) + "]",
		})
	}
	return r, nil
}

func (p *setProxy) A(n int) Proxy {
	a := "[" + strconv.Itoa(n) + "]"
	if n < 0 || n >= len(p.values) {
		return notfoundError(p, a)
	}
	return &valueProxy{
		value:  p.values[n],
		parent: p,
		label:  a,
	}
}

func (p *setProxy) Q(k string) ProxySet {
	w := findAll(p.values, k)
	return &setProxy{
		values: w,
		parent: p,
		label:  ".." + k,
	}
}

func (p *setProxy) Qc(k string) ProxySet {
	r := make([]interface{}, 0, len(p.values))
	for _, v := range p.values {
		switch v := v.(type) {
		case map[string]interface{}:
			if w, ok := v[k]; ok {
				r = append(r, w)
			}
		}
	}
	return &setProxy{
		values: r,
		parent: p,
		label:  ".." + k,
	}
}

func (p *setProxy) parentFrame() frame {
	return p.parent
}

func (p *setProxy) frameLabel() string {
	return p.label
}

func findAll(v interface{}, k string) []interface{} {
	return findAllImpl(v, k, make([]interface{}, 0, 10))
}

func findAllImpl(v interface{}, k string, r []interface{}) []interface{} {
	switch v := v.(type) {
	case map[string]interface{}:
		for n, w := range v {
			if n == k {
				r = append(r, w)
			}
			r = findAllImpl(w, k, r)
		}
	case []interface{}:
		for _, w := range v {
			r = findAllImpl(w, k, r)
		}
	}
	return r
}
