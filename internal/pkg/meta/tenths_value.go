package meta

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
)

type tenthsMeasurement struct {
	tenths int64
}

func newTenthsMeasurement(kind string, value float64) (tenthsMeasurement, error) {
	tenths, err := newTenthsValue(kind, value)
	if err != nil {
		return tenthsMeasurement{}, err
	}
	return newTenthsMeasurementFromTenths(tenths), nil
}

func newTenthsMeasurementFromTenths(tenths int64) tenthsMeasurement {
	return tenthsMeasurement{tenths: tenths}
}

func newTenthsValue(kind string, value float64) (int64, error) {
	if value < 0 {
		return 0, fmt.Errorf("%s must be >= 0", kind)
	}
	return int64(math.Round(value * 10.0)), nil
}

func (m tenthsMeasurement) Float() float64 {
	return tenthsToFloat(m.tenths)
}

func (m tenthsMeasurement) Tenths() int64 {
	return m.tenths
}

func (m tenthsMeasurement) String() string {
	return tenthsToString(m.tenths)
}

func (m tenthsMeasurement) MarshalJSON() ([]byte, error) {
	return marshalTenthsJSON(m.tenths)
}

func (m *tenthsMeasurement) UnmarshalJSONWithKind(kind string, data []byte) error {
	value, err := unmarshalTenthsJSON(kind, data)
	if err != nil {
		return err
	}
	m.tenths = value
	return nil
}

func (m tenthsMeasurement) Value() (driver.Value, error) {
	return tenthsDriverValue(m.tenths)
}

func (m *tenthsMeasurement) ScanWithKind(kind string, src any) error {
	value, err := scanTenthsValue(kind, src)
	if err != nil {
		return err
	}
	m.tenths = value
	return nil
}

func tenthsToFloat(value int64) float64 {
	return float64(value) / 10.0
}

func tenthsToString(value int64) string {
	return fmt.Sprintf("%.1f", tenthsToFloat(value))
}

func marshalTenthsJSON(value int64) ([]byte, error) {
	return json.Marshal(tenthsToFloat(value))
}

func unmarshalTenthsJSON(kind string, data []byte) (int64, error) {
	var value float64
	if err := json.Unmarshal(data, &value); err != nil {
		return 0, fmt.Errorf("invalid %s json", kind)
	}
	return newTenthsValue(kind, value)
}

func scanTenthsValue(kind string, src any) (int64, error) {
	switch value := src.(type) {
	case int64:
		return value, nil
	case float64:
		return newTenthsValue(kind, value)
	default:
		return 0, fmt.Errorf("unsupported scan type %T", src)
	}
}

func tenthsDriverValue(value int64) (driver.Value, error) {
	return value, nil
}
