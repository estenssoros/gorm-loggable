package loggable

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"reflect"
	"sync"

	"github.com/jinzhu/copier"
)

type identityMap map[string]interface{}

var lock sync.Mutex

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
	t := dePointerize(reflect.TypeOf(value))
	newValue := reflect.New(t).Interface()
	if err := copier.Copy(&newValue, value); err != nil {
		return err
	}

	lock.Lock()
	im.m[genIdentityKey(t, pk)] = newValue
	lock.Unlock()

	return nil
}

func (im identityManager) get(value, pk interface{}) interface{} {
	t := dePointerize(reflect.TypeOf(value))
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

func dePointerize(t reflect.Type) reflect.Type {
	for {
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
			continue
		}
		break
	}

	return t
}
