// Code generated by Wire. DO NOT EDIT.

//go:generate go run github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package cases

import (
	"sync"

	"github.com/ecodeclub/ecache"
	"github.com/ecodeclub/webook/internal/cases/internal/repository"
	"github.com/ecodeclub/webook/internal/cases/internal/repository/cache"
	"github.com/ecodeclub/webook/internal/cases/internal/repository/dao"
	"github.com/ecodeclub/webook/internal/cases/internal/service"
	"github.com/ecodeclub/webook/internal/cases/internal/web"
	"github.com/ego-component/egorm"
	"gorm.io/gorm"
)

// Injectors from wire.go:

func InitHandler(db *gorm.DB, ec ecache.Cache) (*web.Handler, error) {
	caseDAO := InitCaseDAO(db)
	caseCache := cache.NewCaseCache(ec)
	caseRepo := repository.NewCaseRepo(caseDAO, caseCache)
	serviceService := service.NewService(caseRepo)
	handler := web.NewHandler(serviceService)
	return handler, nil
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

func InitCaseDAO(db *egorm.Component) dao.CaseDAO {
	InitTableOnce(db)
	return dao.NewCaseDao(db)
}

type Handler = web.Handler