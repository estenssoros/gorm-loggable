package loggable

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/jinzhu/copier"
)

type identityMap map[string]interface{}

// identityManager is used as cache.
type identityManager struct {
	m identityMap
}

func newIdentityManager() *identityManager {
	return &identityManager{
		m: make(identityMap),
	}
}

func (im *identityManager) save(value, pk interface{}) error {
	t := reflect.TypeOf(value)
	newValue := reflect.New(t).Interface()
	if err := copier.Copy(&newValue, value); err != nil {
		return err
	}
	im.m[genIdentityKey(t, pk)] = newValue
	return nil
}

func (im identityManager) get(value, pk interface{}) interface{} {
	t := reflect.TypeOf(value)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	key := genIdentityKey(t, pk)
	m, ok := im.m[key]
	if !ok {
		return nil
	}
	return m
}

func genIdentityKey(t reflect.Type, pk interface{}) string {
	key := fmt.Sprintf("%v_%s", pk, t.Name())
	b := md5.Sum([]byte(key))
	return hex.EncodeToString(b[:])
}
