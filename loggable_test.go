package loggable

import (
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var db *gorm.DB

type SomeType struct {
	gorm.Model
	Source string
	MetaModel
}

type MetaModel struct {
	createdBy string
	LoggableModel
}

func (m MetaModel) Meta() interface{} {
	return struct {
		CreatedBy string
	}{CreatedBy: m.createdBy}
}

func TestMain(m *testing.M) {
	database, err := gorm.Open(
		mysql.Open("constring"),
		&gorm.Config{},
	)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	//database = database.Debug()
	_, err = Register(database)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	err = database.AutoMigrate(SomeType{})
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
	db = database
	os.Exit(m.Run())
}

func TestTryModel(t *testing.T) {
	newmodel := SomeType{Source: time.Now().Format(time.Stamp)}
	newmodel.createdBy = "some user"
	err := db.Create(&newmodel).Error
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(newmodel.ID)
	newmodel.Source = "updated field"
	err = db.Model(SomeType{}).Save(&newmodel).Error
	if err != nil {
		t.Fatal(err)
	}
}
