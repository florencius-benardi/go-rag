package services

import (
	"bytes"
	"encoding/json"
	"go-rag/internal/configs"
	"io"
	"net/http"
	"strconv"

	"gorm.io/gorm"
)

type ServiceContainer struct {
	AppSettings *configs.AppSettings
}

func NewBaseServiceContainer(db *gorm.DB) *ServiceContainer {

	appSettings := &configs.AppSettings{}

	return &ServiceContainer{
		AppSettings: appSettings,
	}
}

type StringInt64 int64

func (s *StringInt64) UnmarshalJSON(data []byte) error {
	*s = StringInt64(parseRawInt64(json.RawMessage(data)))
	return nil
}

func (s StringInt64) MarshalJSON() ([]byte, error) {
	return json.Marshal(int64(s))
}

type IntBool bool

func (s *IntBool) UnmarshalJSON(data []byte) error {
	*s = IntBool(json.RawMessage(data)[0] == 1)
	return nil
}

func (s IntBool) MarshalJSON() ([]byte, error) {
	return json.Marshal(bool(s))
}

type StringBool bool

func (s *StringBool) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		raw = string(data)
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return err
	}
	*s = StringBool(b)
	return nil
}

func (s StringBool) MarshalJSON() ([]byte, error) {
	return json.Marshal(bool(s))
}

type StringFloat64 float64

func (s *StringFloat64) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		raw = string(data)
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return err
	}
	*s = StringFloat64(f)
	return nil
}

func (s StringFloat64) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(s))
}

type NumberString string

func (s *NumberString) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		str = string(data)
	}
	*s = NumberString(str)
	return nil
}

func (s NumberString) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

func parseRawInt64(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		n, _ = strconv.ParseInt(s, 10, 64)
		return n
	}
	return 0
}

func readBody(resp *http.Response) ([]byte, error) {
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// Strip UTF-8 BOM (\xEF\xBB\xBF) that some Windows POS servers prepend
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	// Trim leading whitespace/newlines before JSON parsing
	b = bytes.TrimLeft(b, " \t\r\n")
	return b, nil
}

type ExternalAPIError struct {
	Code    int
	Message string
}

func (e *ExternalAPIError) Error() string {
	return e.Message
}
