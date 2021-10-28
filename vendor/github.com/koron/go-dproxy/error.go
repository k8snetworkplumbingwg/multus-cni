package dproxy

import (
	"fmt"
	"strconv"
)

// ErrorType is type of errors
type ErrorType int

const (
	// Etype means expected type is not matched with actual.
	Etype ErrorType = iota + 1

	// Enotfound means key or index doesn't exist.
	Enotfound

	// EmapNorArray means target is not a map nor an array (for JSON Pointer)
	EmapNorArray

	// EconvertFailure means value conversion is failed.
	EconvertFailure

	// EinvalidIndex means token is invalid as index (for JSON Pointer)
	EinvalidIndex

	// EinvalidQuery means query is invalid as JSON Pointer.
	EinvalidQuery

	// ErequiredType means the type mismatch against user required one.
	// For example M() requires map, A() requires array.
	ErequiredType
)

func (et ErrorType) String() string {
	switch et {
	case Etype:
		return "Etype"
	case Enotfound:
		return "Enotfound"
	case EmapNorArray:
		return "EmapNorArray"
	case EconvertFailure:
		return "EconvertFailure"
	case EinvalidIndex:
		return "EinvalidIndex"
	case EinvalidQuery:
		return "EinvalidQuery"
	case ErequiredType:
		return "ErequiredType"
	default:
		return "Eunknown"
	}
}

// Error get detail information of the errror.
type Error interface {
	// ErrorType returns type of error.
	ErrorType() ErrorType

	// FullAddress returns query string where cause first error.
	FullAddress() string
}

type errorProxy struct {
	errorType ErrorType
	parent    frame
	label     string

	expected Type
	actual   Type
	infoStr  string
}

// errorProxy implements error, Proxy and ProxySet.
var (
	_ error    = (*errorProxy)(nil)
	_ Proxy    = (*errorProxy)(nil)
	_ ProxySet = (*errorProxy)(nil)
)

func (p *errorProxy) Nil() bool {
	return false
}

func (p *errorProxy) Value() (interface{}, error) {
	return nil, p
}

func (p *errorProxy) Bool() (bool, error) {
	return false, p
}

func (p *errorProxy) Int64() (int64, error) {
	return 0, p
}

func (p *errorProxy) Float64() (float64, error) {
	return 0, p
}

func (p *errorProxy) String() (string, error) {
	return "", p
}

func (p *errorProxy) Array() ([]interface{}, error) {
	return nil, p
}

func (p *errorProxy) Map() (map[string]interface{}, error) {
	return nil, p
}

func (p *errorProxy) A(n int) Proxy {
	return p
}

func (p *errorProxy) M(k string) Proxy {
	return p
}

func (p *errorProxy) P(q string) Proxy {
	return p
}

func (p *errorProxy) Empty() bool {
	return true
}

func (p *errorProxy) Len() int {
	return 0
}

func (p *errorProxy) BoolArray() ([]bool, error) {
	return nil, p
}

func (p *errorProxy) Int64Array() ([]int64, error) {
	return nil, p
}

func (p *errorProxy) Float64Array() ([]float64, error) {
	return nil, p
}

func (p *errorProxy) StringArray() ([]string, error) {
	return nil, p
}

func (p *errorProxy) ArrayArray() ([][]interface{}, error) {
	return nil, p
}

func (p *errorProxy) MapArray() ([]map[string]interface{}, error) {
	return nil, p
}

func (p *errorProxy) ProxyArray() ([]Proxy, error) {
	return nil, p
}

func (p *errorProxy) ProxySet() ProxySet {
	return p
}

func (p *errorProxy) Q(k string) ProxySet {
	return p
}

func (p *errorProxy) Qc(k string) ProxySet {
	return p
}

func (p *errorProxy) findJPT(t string) Proxy {
	return p
}

func (p *errorProxy) parentFrame() frame {
	return p.parent
}

func (p *errorProxy) frameLabel() string {
	return p.label
}

func (p *errorProxy) Error() string {
	switch p.errorType {
	case Etype:
		return fmt.Sprintf("not matched types: expected=%s actual=%s: %s",
			p.expected.String(), p.actual.String(), p.FullAddress())
	case Enotfound:
		return fmt.Sprintf("not found: %s", p.FullAddress())
	case EmapNorArray:
		// FIXME: better error message.
		return fmt.Sprintf("not map nor array: actual=%s: %s",
			p.actual.String(), p.FullAddress())
	case EconvertFailure:
		return fmt.Sprintf("convert error: %s: %s", p.infoStr, p.FullAddress())
	case EinvalidIndex:
		// FIXME: better error message.
		return fmt.Sprintf("invalid index: %s: %s", p.infoStr, p.FullAddress())
	case EinvalidQuery:
		// FIXME: better error message.
		return fmt.Sprintf("invalid query: %s: %s", p.infoStr, p.FullAddress())
	case ErequiredType:
		return fmt.Sprintf("not required types: required=%s actual=%s: %s",
			p.expected.String(), p.actual.String(), p.FullAddress())
	default:
		return fmt.Sprintf("unexpected: %s", p.FullAddress())
	}
}

func (p *errorProxy) ErrorType() ErrorType {
	return p.errorType
}

func (p *errorProxy) FullAddress() string {
	return fullAddress(p)
}

func typeError(p frame, expected Type, actual interface{}) *errorProxy {
	return &errorProxy{
		errorType: Etype,
		parent:    p,
		expected:  expected,
		actual:    detectType(actual),
	}
}

func requiredTypeError(p frame, expected Type, actual interface{}) *errorProxy {
	return &errorProxy{
		errorType: ErequiredType,
		parent:    p,
		expected:  expected,
		actual:    detectType(actual),
	}
}

func elementTypeError(p frame, index int, expected Type, actual interface{}) *errorProxy {
	q := &simpleFrame{
		parent: p,
		label:  "[" + strconv.Itoa(index) + "]",
	}
	return typeError(q, expected, actual)
}

func notfoundError(p frame, address string) *errorProxy {
	return &errorProxy{
		errorType: Enotfound,
		parent:    p,
		label:     address,
	}
}
