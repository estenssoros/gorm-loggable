package loggable

import (
	"encoding/json"
	"reflect"

	"github.com/gofrs/uuid"
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
)

var im = newIdentityManager()

const (
	actionCreate = "create"
	actionUpdate = "update"
	actionDelete = "delete"
)

type UpdateDiff map[string]interface{}

// Hook for after_query.
func (p *Plugin) trackEntity(scope *gorm.Scope) {
	// if !isLoggable(scope.Value) || !isEnabled(scope.Value) {
	// 	return
	// }

	v := reflect.Indirect(reflect.ValueOf(scope.Value))

	pkName := scope.PrimaryField().Name
	if v.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			sv := reflect.Indirect(v.Index(i))
			el := sv.Interface()
			if !isLoggable(el) {
				continue
			}
			im.save(el, sv.FieldByName(pkName))
		}
		return
	}

	m := v.Interface()
	if !isLoggable(m) {
		return
	}

	im.save(scope.Value, scope.PrimaryKeyValue())
}

// Hook for after_create.
func (p *Plugin) addCreated(scope *gorm.Scope) {
	if isLoggable(scope.Value) && isEnabled(scope.Value) {
		username, _ := p.context.Value(p.userKey).(string)
		if err := addRecord(scope, actionCreate, username); err != nil {
			scope.DB().AddError(err)
		}
	}
}

// Hook for after_update.
func (p *Plugin) addUpdated(scope *gorm.Scope) {
	if !isLoggable(scope.Value) || !isEnabled(scope.Value) {
		return
	}

	if p.opts.lazyUpdate {
		record, err := p.GetLastRecord(interfaceToString(scope.PrimaryKeyValue()), false)
		if err == nil {
			if isEqual(record.RawObject, scope.Value, p.opts.lazyUpdateFields...) {
				return
			}
		}
		scope.DB().AddError(err)
	}
	username, _ := p.context.Value(p.userKey).(string)
	if err := addUpdateRecord(scope, username, p.opts); err != nil {
		scope.DB().AddError(err)
	}
}

// Hook for after_delete.
func (p *Plugin) addDeleted(scope *gorm.Scope) {
	if isLoggable(scope.Value) && isEnabled(scope.Value) {
		username, _ := p.context.Value(p.userKey).(string)
		if err := addRecord(scope, actionDelete, username); err != nil {
			scope.DB().AddError(err)
		}
	}
}

func addUpdateRecord(scope *gorm.Scope, username string, opts options) error {
	cl, err := newChangeLog(scope, actionUpdate, username)
	if err != nil {
		return err
	}

	if opts.computeDiff {
		diff := computeUpdateDiff(scope)
		if diff != nil {
			jd, err := json.Marshal(diff)
			if err != nil {
				return err
			}

			cl.RawDiff = string(jd)
		}
	}

	return scope.DB().Create(cl).Error
}

func newChangeLog(scope *gorm.Scope, action, username string) (*ChangeLog, error) {
	rawObject, err := json.Marshal(scope.Value)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	meta, err := fetchChangeLogMeta(scope)
	if err != nil {
		return nil, errors.Wrap(err, "fetch changelog meta")
	}
	return &ChangeLog{
		ID:         id,
		UserName:   username,
		Action:     action,
		ObjectID:   interfaceToString(scope.PrimaryKeyValue()),
		ObjectType: scope.GetModelStruct().ModelType.Name(),
		RawObject:  string(rawObject),
		RawMeta:    string(meta),
		RawDiff:    "null",
	}, nil
}

// Writes new change log row to db.
func addRecord(scope *gorm.Scope, action, username string) error {
	cl, err := newChangeLog(scope, action, username)
	if err != nil {
		return errors.Wrap(err, "new change log")
	}
	return scope.DB().Create(cl).Error
}

func computeUpdateDiff(scope *gorm.Scope) UpdateDiff {
	old := im.get(scope.Value, scope.PrimaryKeyValue())
	if old == nil {
		return nil
	}
	ov := reflect.ValueOf(old)
	nv := reflect.Indirect(reflect.ValueOf(scope.Value))
	names := getLoggableFieldNames(old)

	diff := make(UpdateDiff)
	for _, name := range names {
		ofv := ov.FieldByName(name).Interface()
		nfv := nv.FieldByName(name).Interface()
		if ofv != nfv {
			diff[name] = map[string]interface{}{"old": ofv, "new": nfv}
		}
	}
	return diff
}
