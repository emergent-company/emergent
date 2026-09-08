package search

import (
	"go.uber.org/fx"
)

// Module provides search dependencies via fx
var Module = fx.Module("search",
	fx.Provide(
		NewRepository,
		NewTraceStore,
		NewService,
		NewHandler,
	),
	fx.Invoke(RegisterRoutes),
)
