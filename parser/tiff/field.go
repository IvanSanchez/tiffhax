package tiff

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/emilyselwood/tiffhax/parser"
	"github.com/emilyselwood/tiffhax/parser/tiff/constants"
	"github.com/emilyselwood/tiffhax/payload"
	"html/template"
	"io"
)

type Field struct {
	Start    int64
	End      int64
	Data     []byte
	ID       uint16
	DType    uint16
	Count    uint32
	Value    uint32
	IsOffset bool
}

func ParseField(in io.Reader, start int64, order binary.ByteOrder) (*Field, *Offset, *Data, error) {
	data := make([]byte, 12)
	n, err := in.Read(data)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not read ifd field, %v", err)
	}
	if n != 12 {
		return nil, nil, nil, fmt.Errorf("strange size read from ifd field got %v expected 12", n)
	}

	var result Field
	result.Start = start
	result.End = start + 12
	result.Data = data

	// parse the actual data.
	result.ID = order.Uint16(data[0:2])
	result.DType = order.Uint16(data[2:4])
	result.Count = order.Uint32(data[4:8])
	result.Value = order.Uint32(data[8:12])
	// TODO: parse ascii better if its not an offset.

	// do we have an offset or a value
	if result.Count*constants.DataTypeSize[result.DType] > 4 {
		result.IsOffset = true

		var offset Offset
		offset.DType = result.DType
		offset.From = start
		offset.To = int64(result.Value)
		offset.Count = result.Count
		offset.FieldId = result.ID

		if result.ID == 273 || result.ID == 324 {
			offset.IsData = true
		}

		return &result, &offset, nil, nil
	}

	if result.ID == 273 || result.ID == 324 { // stripOffset field wasn't an offset so it must be a single pointer.
		var d Data
		d.Start = int64(result.Value)

		return &result, nil, &d, nil
	}

	return &result, nil, nil, nil
}

func (f *Field) Contains(offset int64) bool {
	return f.Start <= offset && offset < f.End
}

func (f *Field) ContainsRegion(start int64, end int64) bool {
	return f.Start <= start && start < f.End && f.Start < end && end < f.End
}

func (f *Field) Find(offset int64) (parser.Region, error) {
	if offset < f.Start || offset >= f.End {
		return nil, fmt.Errorf("find offset %v outside of field region %v to %v", offset, f.Start, f.End)
	}
	return f, nil
}

func (f *Field) Split(start int64, end int64, newBit parser.Region) error {
	return fmt.Errorf("field can not be split")
}

func (f *Field) Render() ([]payload.Section, error) {

	desc, err := payload.RenderTemplate(fieldTemplate, f, template.FuncMap{
		"FieldNames": func(fieldId uint16) string {
			return constants.FieldNames[fieldId]
		},
		"DataTypeNames": func(typeId uint16) string {
			return constants.DataTypeNames[typeId]
		},
		"FieldValueLookUp" : func() string {
			if f.IsOffset {
				return " which is an offset"
			}

			fieldValues, ok := constants.FieldValueLookup[f.ID]
			if ok {
				value, ok := fieldValues[f.Value]
				if ok {
					return " which means " + value
				}
			}
			if f.DType == 2 {
				value := string(f.Data)
				return " which decodes to " + value
			}
			return ""
		},
		// TODO: better descriptions
	})
	if err != nil {
		return nil, fmt.Errorf("could not format template for field, %v", err)
	}

	var data bytes.Buffer

	payload.RenderBytesSpan(&data, f.Data[0:2], "field_id")
	data.WriteRune(' ')
	payload.RenderBytesSpan(&data, f.Data[2:4], "field_type")
	data.WriteRune(' ')
	payload.RenderBytesSpan(&data, f.Data[4:8], "field_count")
	data.WriteRune(' ')
	payload.RenderBytesSpan(&data, f.Data[8:12], "field_value")

	return []payload.Section{
		&payload.General{
			Start:   f.Start,
			End:     f.End - 1,
			Id:      "ifd_field",
			TheData: template.HTML(data.String()),
			Text:    template.HTML(desc),
		},
	}, nil

}

const fieldTemplate = `A field called <span class="field_id">{{ FieldNames .ID}}</span> 
is <span class="field_count">{{ .Count }}</span> <span class="field_type">{{ DataTypeNames .DType }}</span> values. 
The value shows {{ if .IsOffset }}<a href="#{{ .Value }}">{{end}}<span class="field_value">{{ .Value }}</span>{{ if .IsOffset }}<a/>{{end}}
{{ FieldValueLookUp }}`
