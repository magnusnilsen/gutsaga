// Package addon embeds the GutSaga addon so the companion can install it.
package addon

import "embed"

//go:embed GutSaga/GutSaga.lua GutSaga/GutSagaObjectives.lua GutSaga/GutSagaData.lua GutSaga/GutSaga.tga GutSaga/GutSagaList.tga GutSaga/GutSaga.toc GutSaga/GutSaga_Forever.toc GutSaga/GutSaga_Mainline.toc GutSaga/GutSaga_Vanilla.toc GutSaga/GutSaga_Mists.toc GutSaga/GutSaga_TBC.toc
var Files embed.FS
