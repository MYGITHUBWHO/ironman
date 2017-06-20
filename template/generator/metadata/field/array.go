package field

import "github.com/ironman-project/ironman/text/yaml"

//Array a fixed size array of  fields
type Array struct {
	Field
	size            int
	FieldDefinition interface{} `json:"field_definition" yaml:"field_definition"`
}

//NewArray returns a new initialized array field
func NewArray(f Field, size int, fieldDefinition interface{}) *Array {
	fieldArr := &Array{Field: f, size: size, FieldDefinition: fieldDefinition}
	return fieldArr
}

func (a *Array) String() string {
	return yaml.Print(a)
}