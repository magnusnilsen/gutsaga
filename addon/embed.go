// Package addon embeds the GutSaga addon so the companion can install it.
package addon

import "embed"

//go:embed GutSaga/GutSaga.lua GutSaga/GutSagaObjectives.lua GutSaga/GutSagaData.lua GutSaga/GutSaga.toc GutSaga/GutSaga_Forever.toc GutSaga/GutSaga_Mainline.toc
var Files embed.FS
