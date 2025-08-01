// Code generated by avro/gen. DO NOT EDIT.
package something

import (
	"math/big"
	"time"

	"github.com/taeuk/avro/v2"
)

// ACInnerRecord is a generated struct.
type ACInnerRecord struct {
	InnerJustBytes                   []byte    `avro:"innerJustBytes"`
	InnerPrimitiveNullableArrayUnion *[]string `avro:"innerPrimitiveNullableArrayUnion"`
}

// ABRecordInMap is a generated struct.
type ABRecordInMap struct {
	Name string `avro:"name"`
}

// ABRecordInArray is a generated struct.
type ABRecordInArray struct {
	AString string `avro:"aString"`
}

// ABRecordInNullableUnion is a generated struct.
type ABRecordInNullableUnion struct {
	AString string `avro:"aString"`
}

// ABRecord1InNonNullableUnion is a generated struct.
type ABRecord1InNonNullableUnion struct {
	AString string `avro:"aString"`
}

// ABRecord2InNonNullableUnion is a generated struct.
type ABRecord2InNonNullableUnion struct {
	AString string `avro:"aString"`
}

// ABRecord1InNullableUnion is a generated struct.
type ABRecord1InNullableUnion struct {
	AString string `avro:"aString"`
}

// ABRecord2InNullableUnion is a generated struct.
type ABRecord2InNullableUnion struct {
	AString string `avro:"aString"`
}

// Test represents a golden record.
type ABTest struct {
	// aString is just a string.
	AString string `avro:"aString"`
	// aBoolean is just a boolean.
	ABoolean                        bool                     `avro:"aBoolean"`
	AnInt                           int                      `avro:"anInt"`
	AFloat                          float32                  `avro:"aFloat"`
	ADouble                         float64                  `avro:"aDouble"`
	ALong                           int64                    `avro:"aLong"`
	JustBytes                       []byte                   `avro:"justBytes"`
	PrimitiveNullableArrayUnion     *[]string                `avro:"primitiveNullableArrayUnion"`
	InnerRecord                     ACInnerRecord            `avro:"innerRecord"`
	AnEnum                          string                   `avro:"anEnum"`
	AFixed                          [7]byte                  `avro:"aFixed"`
	ALogicalFixed                   avro.LogicalDuration     `avro:"aLogicalFixed"`
	AnotherLogicalFixed             avro.LogicalDuration     `avro:"anotherLogicalFixed"`
	MapOfStrings                    map[string]string        `avro:"mapOfStrings"`
	MapOfRecords                    map[string]ABRecordInMap `avro:"mapOfRecords"`
	ADate                           time.Time                `avro:"aDate"`
	ADuration                       time.Duration            `avro:"aDuration"`
	ALongTimeMicros                 time.Duration            `avro:"aLongTimeMicros"`
	ALongTimestampMillis            time.Time                `avro:"aLongTimestampMillis"`
	ALongTimestampMicro             time.Time                `avro:"aLongTimestampMicro"`
	ABytesDecimal                   *big.Rat                 `avro:"aBytesDecimal"`
	ARecordArray                    []ABRecordInArray        `avro:"aRecordArray"`
	NullableRecordUnion             *ABRecordInNullableUnion `avro:"nullableRecordUnion"`
	NonNullableRecordUnion          any                      `avro:"nonNullableRecordUnion"`
	NullableRecordUnionWith3Options any                      `avro:"nullableRecordUnionWith3Options"`
	Ref                             ABRecord2InNullableUnion `avro:"ref"`
	UUID                            string                   `avro:"uuid"`
}
