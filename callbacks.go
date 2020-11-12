package loggable

import (
	"encoding/json"
	"math"
	"reflect"
	"time"

	"github.com/estenssoros/dasorm/nulls"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
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

	im.save(db.Statement.ReflectValue.Interface(), scope.PrioritizedPrimaryField.ReflectValueOf(db.Statement.ReflectValue).Interface())
}

// Hook for after_create.
func (p *Plugin) addCreated(db *gorm.DB) {
	v := db.Statement.ReflectValue.Interface()

	if isLoggable(v) && isEnabled(v) {
		old := im.get(v, db.Statement.Schema.PrioritizedPrimaryField.ReflectValueOf(db.Statement.ReflectValue).Interface())
		if old != nil {
			p.addUpdated(db)
			return
		}

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
		record, err := p.GetLastRecord(interfaceToString(scope.PrioritizedPrimaryField.ReflectValueOf(db.Statement.ReflectValue).Interface()), false)
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
	cl, err := newChangeLog(db, actionUpdate, username)
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

	return db.Debug().Exec(cl.Insert()).Error
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
		ObjectID:   interfaceToString(scope.PrioritizedPrimaryField.ReflectValueOf(db.Statement.ReflectValue).Interface()),
		ObjectType: scope.ModelType.Name(),
		RawObject:  string(rawObject),
		RawMeta:    string(meta),
		RawDiff:    "null",
	}, nil
}

// Writes new change log row to db.
func addRecord(db *gorm.DB, action, username string) error {
	cl, err := newChangeLog(db, action, username)
	if err != nil {
		return errors.Wrap(err, "new change log")
	}

	return db.Debug().Exec(cl.Insert()).Error
}

func computeUpdateDiff(db *gorm.DB) UpdateDiff {
	v := db.Statement.ReflectValue.Interface()

	old := im.get(v, db.Statement.Schema.PrioritizedPrimaryField.ReflectValueOf(db.Statement.ReflectValue).Interface())

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
		switch ofv.(type) {
		case nulls.Time:
			if !ofv.(nulls.Time).Time.Equal(nfv.(nulls.Time).Time) {
				diff[name] = map[string]interface{}{"old": ofv, "new": nfv}
			}
		case *time.Time:
			if !ofv.(*time.Time).Equal(*nfv.(*time.Time)) {
				diff[name] = map[string]interface{}{"old": ofv, "new": nfv}
			}
		case nulls.Float64:
			epsilon := .01
			if math.Abs(ofv.(nulls.Float64).Float64-nfv.(nulls.Float64).Float64) > epsilon {
				diff[name] = map[string]interface{}{"old": ofv, "new": nfv}
			}
		case float64:
			epsilon := .01
			if math.Abs(ofv.(float64)-nfv.(float64)) > epsilon {
				diff[name] = map[string]interface{}{"old": ofv, "new": nfv}
			}
		default:
			if ofv != nfv {
				diff[name] = map[string]interface{}{"old": ofv, "new": nfv}
			}
		}

	}

	if len(diff) == 0 {
		return nil
	}

	return diff
}
