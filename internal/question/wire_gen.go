// Code generated by Wire. DO NOT EDIT.

//go:generate go run github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package baguwen

import (
	"sync"

	"github.com/ecodeclub/ecache"
	"github.com/ecodeclub/webook/internal/question/internal/repository"
	"github.com/ecodeclub/webook/internal/question/internal/repository/cache"
	"github.com/ecodeclub/webook/internal/question/internal/repository/dao"
	"github.com/ecodeclub/webook/internal/question/internal/service"
	"github.com/ecodeclub/webook/internal/question/internal/web"
	"github.com/ego-component/egorm"
	"gorm.io/gorm"
)

// Injectors from wire.go:

func InitModule(db *gorm.DB, ec ecache.Cache) (*Module, error) {
	questionDAO := InitQuestionDAO(db)
	questionCache := cache.NewQuestionECache(ec)
	repositoryRepository := repository.NewCacheRepository(questionDAO, questionCache)
	serviceService := service.NewService(repositoryRepository)
	handler := web.NewHandler(serviceService)
	questionSetDAO := InitQuestionSetDAO(db)
	questionSetRepository := repository.NewQuestionSetRepository(questionSetDAO)
	questionSetService := service.NewQuestionSetService(questionSetRepository)
	questionSetHandler, err := web.NewQuestionSetHandler(questionSetService)
	if err != nil {
		return nil, err
	}
	module := &Module{
		Svc:   serviceService,
		Hdl:   handler,
		QsHdl: questionSetHandler,
	}
	return module, nil
}

// wire.go:

var daoOnce = sync.Once{}

func InitTableOnce(db *gorm.DB) {
	daoOnce.Do(func() {
		err := dao.InitTables(db)
		if err != nil {
			panic(err)
		}
	})
}

func InitQuestionDAO(db *egorm.Component) dao.QuestionDAO {
	InitTableOnce(db)
	return dao.NewGORMQuestionDAO(db)
}

func InitQuestionSetDAO(db *egorm.Component) dao.QuestionSetDAO {
	InitTableOnce(db)
	return dao.NewGORMQuestionSetDAO(db)
}
