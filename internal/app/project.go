package app

import (
	"context"
	"fmt"

	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/handler"
	"audio-ui/internal/infra/persistence"
	"audio-ui/internal/service/project"

	"go.uber.org/zap"
)

type projectSetup struct {
	handler *handler.ProjectService
	service *project.Service
}

func newProjectSetup(layout valueobject.Layout, logger *zap.Logger, onChange ...func(string, int64)) *projectSetup {
	configStore := persistence.NewConfigStore(layout.ConfigFile)
	projectStore := persistence.NewProjectStore(layout)
	migrator := persistence.NewM1Migrator(layout)
	var changed func(string, int64)
	if len(onChange) > 0 {
		changed = onChange[0]
	}
	service := project.New(project.Options{
		Layout:   layout,
		Config:   configStore,
		Projects: projectStore,
		Migrator: migrator,
		OnChange: changed,
		Logf: func(format string, args ...any) {
			logger.Info("project", zap.String("detail", fmt.Sprintf(format, args...)))
		},
	})
	if err := service.Start(context.Background()); err != nil {
		logger.Error("project startup failed", zap.Error(err))
	}
	return &projectSetup{
		handler: handler.NewProjectService(service),
		service: service,
	}
}
