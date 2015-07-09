package snooze

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"
)

type Client struct {
	Before func(*http.Request, *http.Client)
	Root   string
}

type resultInfo struct {
	errorIndex   int
	payloadIndex int
	payloadType  reflect.Type
	resultLength int
}

func (info *resultInfo) result(err error, bytes []byte) []reflect.Value {
	result := make([]reflect.Value, info.resultLength)
	if info.errorIndex > -1 {
		if err != nil {
			result[info.errorIndex] = reflect.ValueOf(&err).Elem()
		} else {
			result[info.errorIndex] = nilError
		}
	}
	if info.payloadIndex > -1 {
		if bytes != nil {
			target := reflect.New(info.payloadType)
			err = json.Unmarshal(bytes, target.Interface())
			if err != nil {
				return info.result(err, nil)
			}
			result[info.payloadIndex] = target.Elem()
		} else {
			result[info.payloadIndex] = reflect.Zero(info.payloadType)
		}
	}
	return result
}

var nilError = reflect.Zero(reflect.TypeOf((*error)(nil)).Elem())

func (c *Client) Create(in interface{}) {
	inputValue := reflect.ValueOf(in).Elem()
	inputType := inputValue.Type()
	for i := 0; i < inputValue.NumField(); i++ {
		fieldValue := inputValue.Field(i)
		fieldStruct := inputType.Field(i)
		fieldType := fieldStruct.Type
		originalPath := fieldStruct.Tag.Get("path")
		method := fieldStruct.Tag.Get("method")
		var body interface{}

		info := resultInfo{
			resultLength: fieldType.NumOut(),
			errorIndex:   -1,
			payloadIndex: -1,
		}
		for n := 0; n < info.resultLength; n++ {
			out := fieldType.Out(n)
			if out == reflect.TypeOf((*error)(nil)).Elem() {
				info.errorIndex = n
			} else {
				info.payloadIndex = n
				info.payloadType = out
			}
		}

		fieldValue.Set(reflect.MakeFunc(fieldType, func(args []reflect.Value) []reflect.Value {
			path := originalPath
			for n, av := range args {
				if av.Kind() == reflect.Struct {
					body = av.Interface()
					continue
				}
				path = strings.Replace(path, fmt.Sprintf("{%v}", n), url.QueryEscape(fmt.Sprint(av.Interface())), -1)
			}

			var err error
			buffer := make([]byte, 0)
			if method != "GET" && body != nil {
				buffer, err = json.Marshal(body)
				if err != nil {
					return info.result(err, nil)
				}
			}

			req, err := http.NewRequest(method, c.Root+path, bytes.NewBuffer(buffer))
			if err != nil {
				return info.result(err, nil)
			}
			req.Header.Set("Content-Type", "application/json")
			client := new(http.Client)
			if c.Before != nil {
				c.Before(req, client)
			}
			resp, err := client.Do(req)
			if err != nil {
				return info.result(err, nil)
			}
			bytes, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return info.result(err, nil)
			}
			return info.result(nil, bytes)
		}))
	}
}
