// Code generated by Wire. DO NOT EDIT.

//go:generate go run github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package baguwen

import (
	"github.com/ecodeclub/webook/internal/question/internal/repository"
	"github.com/ecodeclub/webook/internal/question/internal/repository/dao"
	"github.com/ecodeclub/webook/internal/question/internal/service"
	"github.com/ecodeclub/webook/internal/question/internal/web"
	"gorm.io/gorm"
)

// Injectors from wire.go:

func InitHandler(db *gorm.DB) (*web.Handler, error) {
	questionDAO := dao.NewGORMQuestionDAO(db)
	repositoryRepository := repository.NewCacheRepository(questionDAO)
	serviceService := service.NewService(repositoryRepository)
	handler, err := web.NewHandler(serviceService)
	if err != nil {
		return nil, err
	}
	return handler, nil
}
