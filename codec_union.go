package avro

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unsafe"

	"github.com/modern-go/reflect2"
)

// UnionConverter to handle Avro Union's in a type-safe way.
type UnionConverter interface {
	// FromAny payload decode into any of the mentioned types in the Union.
	FromAny(payload any) error
	// ToAny from the Union struct
	ToAny() (any, error)
}

func createDecoderOfUnion(d *decoderContext, schema *UnionSchema, typ reflect2.Type) ValDecoder {
	switch typ.Kind() {
	case reflect.Map:
		if typ.(reflect2.MapType).Key().Kind() != reflect.String ||
			typ.(reflect2.MapType).Elem().Kind() != reflect.Interface {
			break
		}
		return decoderOfMapUnion(d, schema, typ)
	case reflect.Slice:
		if !schema.Nullable() {
			break
		}
		return decoderOfNullableUnion(d, schema, typ)
	case reflect.Ptr:
		if typ.Implements(reflect2.Type2(reflect.TypeFor[UnionConverter]())) {
			return decoderOfUnionConverterCodec(d, schema, typ)
		}

		if !schema.Nullable() {
			break
		}
		return decoderOfNullableUnion(d, schema, typ)
	case reflect.Interface:
		if _, ok := typ.(*reflect2.UnsafeIFaceType); !ok {
			dec, err := decoderOfResolvedUnion(d, schema, typ)
			if err != nil {
				return &errorDecoder{err: fmt.Errorf("avro: problem resolving decoder for Avro %s: %w", schema.Type(), err)}
			}
			return dec
		}
	case reflect.Struct:
		return createDecoderOfUnion(d, schema, reflect2.PtrTo(typ))
	}

	return &errorDecoder{err: fmt.Errorf("avro: %s is unsupported for Avro %s", typ.String(), schema.Type())}
}

func createEncoderOfUnion(e *encoderContext, schema *UnionSchema, typ reflect2.Type) ValEncoder {
	switch typ.Kind() {
	case reflect.Map:
		if typ.(reflect2.MapType).Key().Kind() != reflect.String ||
			typ.(reflect2.MapType).Elem().Kind() != reflect.Interface {
			break
		}
		return encoderOfMapUnion(e, schema, typ)
	case reflect.Slice:
		if !schema.Nullable() {
			break
		}
		return encoderOfNullableUnion(e, schema, typ)
	case reflect.Ptr:
		if typ.Implements(reflect2.Type2(reflect.TypeFor[UnionConverter]())) {
			return encoderOfUnionConverterCodec(e, schema, typ)
		}

		if !schema.Nullable() {
			break
		}
		return encoderOfNullableUnion(e, schema, typ)
	}

	return encoderOfResolverUnion(e, schema, typ)
}

func decoderOfMapUnion(d *decoderContext, union *UnionSchema, typ reflect2.Type) ValDecoder {
	mapType := typ.(*reflect2.UnsafeMapType)

	typeDecs := make([]ValDecoder, len(union.Types()))
	for i, s := range union.Types() {
		if s.Type() == Null {
			continue
		}
		typeDecs[i] = newEfaceDecoder(d, s)
	}

	return &mapUnionDecoder{
		cfg:      d.cfg,
		schema:   union,
		mapType:  mapType,
		elemType: mapType.Elem(),
		typeDecs: typeDecs,
	}
}

type mapUnionDecoder struct {
	cfg      *frozenConfig
	schema   *UnionSchema
	mapType  *reflect2.UnsafeMapType
	elemType reflect2.Type
	typeDecs []ValDecoder
}

func (d *mapUnionDecoder) Decode(ptr unsafe.Pointer, r *Reader) {
	idx, resSchema := getUnionSchema(d.schema, r)
	if resSchema == nil {
		return
	}

	// In a null case, just return
	if resSchema.Type() == Null {
		return
	}

	if d.mapType.UnsafeIsNil(ptr) {
		d.mapType.UnsafeSet(ptr, d.mapType.UnsafeMakeMap(1))
	}

	key := schemaTypeName(resSchema)
	keyPtr := reflect2.PtrOf(key)

	elemPtr := d.elemType.UnsafeNew()
	d.typeDecs[idx].Decode(elemPtr, r)

	d.mapType.UnsafeSetIndex(ptr, keyPtr, elemPtr)
}

func encoderOfMapUnion(e *encoderContext, union *UnionSchema, _ reflect2.Type) ValEncoder {
	return &mapUnionEncoder{
		cfg:    e.cfg,
		schema: union,
	}
}

type mapUnionEncoder struct {
	cfg    *frozenConfig
	schema *UnionSchema
}

func (e *mapUnionEncoder) Encode(ptr unsafe.Pointer, w *Writer) {
	m := *((*map[string]any)(ptr))

	if len(m) > 1 {
		w.Error = errors.New("avro: cannot encode union map with multiple entries")
		return
	}

	name := "null"
	val := any(nil)
	for k, v := range m {
		name = k
		val = v
		break
	}

	schema, pos := e.schema.Types().Get(name)
	if schema == nil {
		w.Error = fmt.Errorf("avro: unknown union type %s", name)
		return
	}

	w.WriteInt(int32(pos))

	if schema.Type() == Null && val == nil {
		return
	}

	// encode a nil slice as an empty array
	if schema.Type() == Array && val == nil {
		// element data type doesn't matter since it skips iterating the slice
		val = []struct{}{}
	}

	val, err := w.cfg.typeConverters.EncodeTypeConvert(val, e.schema)
	if err != nil && !errors.Is(err, errNoTypeConverter) {
		w.Error = err
		return
	}

	elemType := reflect2.TypeOf(val)
	elemPtr := reflect2.PtrOf(val)

	encoder := encoderOfType(newEncoderContext(e.cfg), schema, elemType)
	if elemType.LikePtr() {
		encoder = &onePtrEncoder{encoder}
	}
	encoder.Encode(elemPtr, w)
}

func decoderOfNullableUnion(d *decoderContext, schema Schema, typ reflect2.Type) ValDecoder {
	union := schema.(*UnionSchema)
	_, typeIdx := union.Indices()

	var (
		baseTyp reflect2.Type
		isPtr   bool
	)
	switch v := typ.(type) {
	case *reflect2.UnsafePtrType:
		baseTyp = v.Elem()
		isPtr = true
	case *reflect2.UnsafeSliceType:
		baseTyp = v
	}
	decoder := decoderOfType(d, union.Types()[typeIdx], baseTyp)

	return &unionNullableDecoder{
		schema:  union,
		typ:     baseTyp,
		isPtr:   isPtr,
		decoder: decoder,
	}
}

type unionNullableDecoder struct {
	schema  *UnionSchema
	typ     reflect2.Type
	isPtr   bool
	decoder ValDecoder
}

func (d *unionNullableDecoder) Decode(ptr unsafe.Pointer, r *Reader) {
	_, schema := getUnionSchema(d.schema, r)
	if schema == nil {
		return
	}

	if schema.Type() == Null {
		*((*unsafe.Pointer)(ptr)) = nil
		return
	}

	defer func() {
		if !d.isPtr {
			obj := d.typ.UnsafeIndirect(ptr)
			obj, err := r.cfg.typeConverters.DecodeTypeConvert(obj, d.schema)
			if errors.Is(err, errNoTypeConverter) {
				return
			}
			if err != nil {
				r.Error = err
			}
			if obj == nil {
				*(*unsafe.Pointer)(ptr) = nil
				return
			}
			d.typ.UnsafeSet(ptr, reflect2.PtrOf(obj))
			return
		}
		obj := d.typ.UnsafeIndirect(*((*unsafe.Pointer)(ptr)))
		obj, err := r.cfg.typeConverters.DecodeTypeConvert(obj, d.schema)
		if errors.Is(err, errNoTypeConverter) {
			return
		}
		if err != nil {
			r.Error = err
		}
		*((*unsafe.Pointer)(ptr)) = reflect2.PtrOf(obj)
	}()

	// Handle the non-ptr case separately.
	if !d.isPtr {
		if d.typ.UnsafeIsNil(ptr) {
			// Create a new instance.
			newPtr := d.typ.UnsafeNew()
			d.decoder.Decode(newPtr, r)
			d.typ.UnsafeSet(ptr, newPtr)
			return
		}

		// Reuse the existing instance.
		d.decoder.Decode(ptr, r)
		return
	}

	if *((*unsafe.Pointer)(ptr)) == nil {
		// Create new instance.
		newPtr := d.typ.UnsafeNew()
		d.decoder.Decode(newPtr, r)
		*((*unsafe.Pointer)(ptr)) = newPtr
		return
	}

	// Reuse existing instance.
	d.decoder.Decode(*((*unsafe.Pointer)(ptr)), r)
}

func encoderOfUnionConverterCodec(_ *encoderContext, schema Schema, typ reflect2.Type) ValEncoder {
	union := schema.(*UnionSchema)
	var nullIdx int32
	var nullable bool

	for i, unionSchema := range union.Types() {
		if unionSchema.Type() == Null {
			nullIdx = int32(i)
			nullable = true
		}
	}

	return &unionConverterToAnyCodec{
		schema:   union,
		typ:      typ,
		nullable: nullable,
		nullIdx:  nullIdx,
	}
}

type unionConverterToAnyCodec struct {
	schema   *UnionSchema
	typ      reflect2.Type
	nullable bool
	nullIdx  int32
}

func (e *unionConverterToAnyCodec) Encode(ptr unsafe.Pointer, w *Writer) {
	if *((*unsafe.Pointer)(ptr)) == nil {
		if !e.nullable {
			w.Error = errors.New("avro: unionConverterToAnyCodec: encoding nil value for non nillable union")
			return
		}
		w.WriteInt(e.nullIdx)
		return
	}

	target := e.typ.UnsafeIndirect(ptr)
	marshaller := target.(UnionConverter)
	val, err := marshaller.ToAny()
	if err != nil {
		w.Error = fmt.Errorf("avro: unable to convert union: %w", err)
		return
	}

	typeOf := reflect2.TypeOf(val)
	typeOfUnsafePtr, ok := typeOf.(*reflect2.UnsafePtrType)
	if !ok {
		w.Error = fmt.Errorf("avro: expected ptr but received %q", typeOf.String())
		return
	}

	elemType := typeOfUnsafePtr.Elem()
	w.WriteVal(e.schema, elemType.Indirect(val))
}

func encoderOfNullableUnion(e *encoderContext, schema Schema, typ reflect2.Type) ValEncoder {
	union := schema.(*UnionSchema)
	nullIdx, typeIdx := union.Indices()

	var (
		baseTyp reflect2.Type
		isPtr   bool
	)
	switch v := typ.(type) {
	case *reflect2.UnsafePtrType:
		baseTyp = v.Elem()
		isPtr = true
	case *reflect2.UnsafeSliceType:
		baseTyp = v
	}
	encoder := encoderOfType(e, union.Types()[typeIdx], baseTyp)

	return &unionNullableEncoder{
		schema:  union,
		encoder: encoder,
		isPtr:   isPtr,
		nullIdx: int32(nullIdx),
		typeIdx: int32(typeIdx),
	}
}

type unionNullableEncoder struct {
	schema  *UnionSchema
	encoder ValEncoder
	isPtr   bool
	nullIdx int32
	typeIdx int32
}

func (e *unionNullableEncoder) Encode(ptr unsafe.Pointer, w *Writer) {
	if *((*unsafe.Pointer)(ptr)) == nil {
		w.WriteInt(e.nullIdx)
		return
	}

	w.WriteInt(e.typeIdx)
	newPtr := ptr
	if e.isPtr {
		newPtr = *((*unsafe.Pointer)(ptr))
	}
	e.encoder.Encode(newPtr, w)
}

func decoderOfResolvedUnion(d *decoderContext, schema Schema, _ reflect2.Type) (ValDecoder, error) {
	union := schema.(*UnionSchema)

	types := make([]reflect2.Type, len(union.Types()))
	decoders := make([]ValDecoder, len(union.Types()))
	for i, schema := range union.Types() {
		name := unionResolutionName(schema)

		typ, err := d.cfg.resolver.Type(name)
		if err != nil {
			if d.cfg.config.UnionResolutionError {
				return nil, err
			}

			if d.cfg.config.PartialUnionTypeResolution {
				decoders[i] = nil
				types[i] = nil
				continue
			}

			decoders = []ValDecoder{}
			types = []reflect2.Type{}
			break
		}

		decoder := decoderOfType(d, schema, typ)
		decoders[i] = decoder
		types[i] = typ
	}

	return &unionResolvedDecoder{
		cfg:      d.cfg,
		schema:   union,
		types:    types,
		decoders: decoders,
	}, nil
}

type unionResolvedDecoder struct {
	cfg      *frozenConfig
	schema   *UnionSchema
	types    []reflect2.Type
	decoders []ValDecoder
}

func (d *unionResolvedDecoder) Decode(ptr unsafe.Pointer, r *Reader) {
	i, schema := getUnionSchema(d.schema, r)
	if schema == nil {
		return
	}

	pObj := (*any)(ptr)

	if schema.Type() == Null {
		*pObj = nil
		return
	}

	defer func() {
		obj, err := r.cfg.typeConverters.DecodeTypeConvert(*pObj, d.schema)
		if err != nil && !errors.Is(err, errNoTypeConverter) {
			r.Error = err
		}
		*pObj = obj
	}()

	if i >= len(d.decoders) || d.decoders[i] == nil {
		if d.cfg.config.UnionResolutionError {
			r.ReportError("decode union type", "unknown union type")
			return
		}

		// We cannot resolve this, set it to the map type
		name := schemaTypeName(schema)
		obj := map[string]any{}
		vTyp, err := genericReceiver(schema)
		if err != nil {
			r.ReportError("Union", err.Error())
			return
		}
		obj[name] = genericDecode(vTyp, decoderOfType(newDecoderContext(d.cfg), schema, vTyp), r)

		*pObj = obj
		return
	}

	typ := d.types[i]
	var newPtr unsafe.Pointer
	switch typ.Kind() {
	case reflect.Map:
		mapType := typ.(*reflect2.UnsafeMapType)
		newPtr = mapType.UnsafeMakeMap(0)

	case reflect.Slice:
		mapType := typ.(*reflect2.UnsafeSliceType)
		newPtr = mapType.UnsafeMakeSlice(0, 0)

	case reflect.Ptr:
		elemType := typ.(*reflect2.UnsafePtrType).Elem()
		newPtr = elemType.UnsafeNew()

	default:
		newPtr = typ.UnsafeNew()
	}

	d.decoders[i].Decode(newPtr, r)

	*pObj = typ.UnsafeIndirect(newPtr)
}

func decoderOfUnionConverterCodec(d *decoderContext, schema *UnionSchema, typ reflect2.Type) ValDecoder {
	anyDecoder := createDecoderOfUnion(d, schema, reflect2.Type2(reflect.TypeFor[any]()))
	nullable := slices.ContainsFunc(schema.Types(), func(schema Schema) bool {
		return schema.Type() == Null
	})

	return &unionConverterFromAnyCodec{
		decoder:  anyDecoder,
		schema:   schema,
		nullable: nullable,
		typ:      typ,
	}
}

type unionConverterFromAnyCodec struct {
	decoder  ValDecoder
	schema   *UnionSchema
	nullable bool
	typ      reflect2.Type
}

func (d *unionConverterFromAnyCodec) Decode(ptr unsafe.Pointer, r *Reader) {
	obj := new(any)
	newPtr := reflect2.PtrOf(obj)
	d.decoder.Decode(newPtr, r)

	if *obj == nil {
		if d.nullable {
			return
		}

		r.Error = errors.New("avro: cannot decode nil value in non-nullable union type")
		return
	}

	if d.typ.Kind() == reflect.Ptr {
		ptrType := d.typ.(*reflect2.UnsafePtrType).Elem()
		elemPtr := ptrType.UnsafeNew()
		*((*unsafe.Pointer)(ptr)) = elemPtr
	}
	target := d.typ.UnsafeIndirect(ptr)

	unionConverter := target.(UnionConverter)
	if err := unionConverter.FromAny(*obj); err != nil {
		r.ReportError("Union", err.Error())
		return
	}
}

func unionResolutionName(schema Schema) string {
	name := schemaTypeName(schema)
	switch schema.Type() {
	case Map:
		name += ":"
		valSchema := schema.(*MapSchema).Values()
		valName := schemaTypeName(valSchema)

		name += valName

	case Array:
		name += ":"
		itemSchema := schema.(*ArraySchema).Items()
		itemName := schemaTypeName(itemSchema)

		name += itemName
	}

	return name
}

func encoderOfResolverUnion(e *encoderContext, schema Schema, typ reflect2.Type) ValEncoder {
	union := schema.(*UnionSchema)

	names, err := e.cfg.resolver.Name(typ)
	if err != nil {
		return &errorEncoder{err: err}
	}

	var pos int
	for _, name := range names {
		if idx := strings.Index(name, ":"); idx > 0 {
			name = name[:idx]
		}

		schema, pos = union.Types().Get(name)
		if schema != nil {
			break
		}
	}
	if schema == nil {
		return &errorEncoder{err: fmt.Errorf("avro: unknown union type %s", names[0])}
	}

	encoder := encoderOfType(e, schema, typ)

	return &unionResolverEncoder{
		pos:     pos,
		encoder: encoder,
	}
}

type unionResolverEncoder struct {
	pos     int
	encoder ValEncoder
}

func (e *unionResolverEncoder) Encode(ptr unsafe.Pointer, w *Writer) {
	w.WriteInt(int32(e.pos))

	e.encoder.Encode(ptr, w)
}

func getUnionSchema(schema *UnionSchema, r *Reader) (int, Schema) {
	types := schema.Types()

	idx := int(r.ReadInt())
	if idx < 0 || idx > len(types)-1 {
		r.ReportError("decode union type", "unknown union type")
		return 0, nil
	}

	return idx, types[idx]
}
