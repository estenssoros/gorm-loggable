package loggable

import (
	"encoding/json"
	"reflect"

	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

var im = newIdentityManager()

const (
	actionCreate = "create"
	actionUpdate = "update"
	actionDelete = "delete"
)

type UpdateDiff map[string]interface{}

// Hook for after_query.
func (p *Plugin) trackEntity(db *gorm.DB) {
	scope := db.Statement.Schema
	// if !isLoggable(scope.Value) || !isEnabled(scope.Value) {
	// 	return
	// }

	v := reflect.Indirect(db.Statement.ReflectValue)

	if scope.PrioritizedPrimaryField == nil {
		return
	}

	pkName := scope.PrioritizedPrimaryField.Name
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

	im.save(db.Statement.ReflectValue, scope.PrioritizedPrimaryField.ReflectValueOf)
}

// Hook for after_create.
func (p *Plugin) addCreated(db *gorm.DB) {
	v := db.Statement.ReflectValue.Interface()

	if isLoggable(v) && isEnabled(v) {
		username, _ := p.Context.Value(p.userKey).(string)
		if err := addRecord(db, actionCreate, username); err != nil {
			db.AddError(err)
		}
	}
}

// Hook for after_update.
func (p *Plugin) addUpdated(db *gorm.DB) {
	v := db.Statement.ReflectValue.Interface()
	scope := db.Statement.Schema

	if !isLoggable(v) || !isEnabled(v) {
		return
	}

	if p.opts.lazyUpdate {
		record, err := p.GetLastRecord(interfaceToString(scope.PrioritizedPrimaryField.ReflectValueOf), false)
		if err == nil {
			if isEqual(record.RawObject, v, p.opts.lazyUpdateFields...) {
				return
			}
		}
		db.AddError(err)
	}
	username, _ := p.Context.Value(p.userKey).(string)
	if err := addUpdateRecord(db, username, p.opts); err != nil {
		db.AddError(err)
	}
}

func addUpdateRecord(db *gorm.DB, username string, opts options) error {
	//logrus.Info("update")
	cl, err := newChangeLog(db, actionUpdate, username)
	//logrus.Infof("%+v\n::%s", cl, "update")
	if err != nil {
		return err
	}

	if opts.computeDiff {
		diff := computeUpdateDiff(db)
		if len(diff) == 0 {
			return nil
		}
		if diff != nil {
			jd, err := json.Marshal(diff)
			if err != nil {
				return err
			}

			cl.RawDiff = string(jd)
		}
	}

	err = db.Create(&cl).Error
	//logrus.Info(err)

	return err
}

func newChangeLog(db *gorm.DB, action, username string) (*ChangeLog, error) {
	v := db.Statement.ReflectValue.Interface()
	scope := db.Statement.Schema

	rawObject, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}
	meta, err := fetchChangeLogMeta(db)
	if err != nil {
		return nil, errors.Wrap(err, "fetch changelog meta")
	}
	return &ChangeLog{
		ID:         id,
		UserName:   username,
		Action:     action,
		ObjectID:   interfaceToString(scope.PrioritizedPrimaryField.ReflectValueOf),
		ObjectType: scope.ModelType.Name(),
		RawObject:  string(rawObject),
		RawMeta:    string(meta),
		RawDiff:    "null",
	}, nil
}

// Writes new change log row to db.
func addRecord(db *gorm.DB, action, username string) error {
	logrus.Info("here")
	cl, err := newChangeLog(db, action, username)
	logrus.Infof("%+v\n::%s", cl, action)
	if err != nil {
		return errors.Wrap(err, "new change log")
	}

	err = db.Create(&cl).Error
	logrus.Info(err)
	return err
}

func computeUpdateDiff(db *gorm.DB) UpdateDiff {
	v := db.Statement.ReflectValue.Interface()

	old := im.get(v, db.Statement.Schema.PrioritizedPrimaryField.ReflectValueOf)
	if old == nil {
		return nil
	}
	ov := reflect.ValueOf(old)
	nv := reflect.Indirect(reflect.ValueOf(v))
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
