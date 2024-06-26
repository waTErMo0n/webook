// Code generated by Wire. DO NOT EDIT.

//go:generate go run github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package member

import (
	"context"
	"sync"

	"github.com/ecodeclub/mq-api"
	"github.com/ecodeclub/webook/internal/member/internal/domain"
	"github.com/ecodeclub/webook/internal/member/internal/event"
	"github.com/ecodeclub/webook/internal/member/internal/repository"
	"github.com/ecodeclub/webook/internal/member/internal/repository/dao"
	"github.com/ecodeclub/webook/internal/member/internal/service"
	"github.com/ego-component/egorm"
	"gorm.io/gorm"
)

// Injectors from wire.go:

func InitModule(db *gorm.DB, q mq.MQ) (*Module, error) {
	service := InitService(db, q)
	registrationEventConsumer := initRegistrationConsumer(service, q)
	module := &Module{
		Svc: service,
		c:   registrationEventConsumer,
	}
	return module, nil
}

// wire.go:

type Member = domain.Member

type Service = service.Service

var (
	once = &sync.Once{}
	svc  service.Service
)

func InitService(db *egorm.Component, q mq.MQ) Service {
	once.Do(func() {
		_ = dao.InitTables(db)
		d := dao.NewMemberGORMDAO(db)
		r := repository.NewMemberRepository(d)
		svc = service.NewMemberService(r)
	})
	return svc
}

func initRegistrationConsumer(svc2 service.Service, q mq.MQ) *event.RegistrationEventConsumer {
	c, err := event.NewRegistrationEventConsumer(svc2, q)
	if err != nil {
		panic(err)
	}
	c.Start(context.Background())
	return c
}
