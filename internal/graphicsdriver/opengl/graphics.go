// Copyright 2018 The Ebiten Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !playstation5

package opengl

import (
	"fmt"
	"math"
	"unsafe"

	"github.com/hajimehoshi/ebiten/v2/internal/graphics"
	"github.com/hajimehoshi/ebiten/v2/internal/graphicsdriver"
	"github.com/hajimehoshi/ebiten/v2/internal/graphicsdriver/opengl/gl"
	"github.com/hajimehoshi/ebiten/v2/internal/shaderir"
)

type activatedTexture struct {
	textureNative textureNative
	index         int
}

// Values for the ebiten_3d_adjust vertex-shader uniform (see the GLSL
// compiler). Kept in two distinct immutable arrays so the uniform cache's
// change detection (which compares slice contents) works across draws that
// alternate between 2D and 3D modes.
var (
	adjust3DOff = [1]uint32{0}
	adjust3DOn  = [1]uint32{math.Float32bits(1)}
)

var (
	_ graphicsdriver.DepthTextureAttacher  = (*Graphics)(nil)
	_ graphicsdriver.DrawTrianglesWithMode = (*Graphics)(nil)
)

type Graphics struct {
	state   openGLState
	context context
	vsync   bool

	nextImageID graphicsdriver.ImageID
	images      map[graphicsdriver.ImageID]*Image

	nextShaderID graphicsdriver.ShaderID
	shaders      map[graphicsdriver.ShaderID]*Shader

	// drawCalled is true just after Draw is called. This holds true until WritePixels is called.
	drawCalled bool

	frame int64

	uniformVariableNameCache map[int]string
	textureVariableNameCache map[int]string

	uniformVars []uniformVariable

	// activatedTextures is a set of activated textures.
	// textureNative cannot be a map key unfortunately.
	activatedTextures []activatedTexture

	graphicsPlatform
}

func newGraphics(ctx gl.Context) *Graphics {
	g := &Graphics{
		vsync: true,
	}
	if isDebug {
		g.context.ctx = &gl.DebugContext{Context: ctx}
	} else {
		g.context.ctx = ctx
	}
	return g
}

func (g *Graphics) Begin() error {
	// Do nothing.
	return nil
}

func (g *Graphics) End(present bool) error {
	// Call glFlush to prevent black flicking (especially on Android (#226) and iOS).
	// TODO: examples/sprites worked without this. Is this really needed?
	g.context.ctx.Flush()

	// The last uniforms must be reset before swapping the buffer (#2517).
	if present {
		g.state.resetLastUniforms()
		if err := g.swapBuffers(); err != nil {
			return err
		}
		g.frame++
	}

	return nil
}

func (g *Graphics) SetTransparent(transparent bool) {
	// Do nothing.
}

func (g *Graphics) checkSize(width, height int) {
	if width < 1 {
		panic(fmt.Sprintf("opengl: width (%d) must be equal or more than %d", width, 1))
	}
	if height < 1 {
		panic(fmt.Sprintf("opengl: height (%d) must be equal or more than %d", height, 1))
	}
	m := g.context.getMaxTextureSize()
	if width > m {
		panic(fmt.Sprintf("opengl: width (%d) must be less than or equal to %d", width, m))
	}
	if height > m {
		panic(fmt.Sprintf("opengl: height (%d) must be less than or equal to %d", height, m))
	}
}

func (g *Graphics) genNextImageID() graphicsdriver.ImageID {
	g.nextImageID++
	return g.nextImageID
}

func (g *Graphics) genNextShaderID() graphicsdriver.ShaderID {
	g.nextShaderID++
	return g.nextShaderID
}

func (g *Graphics) NewImage(width, height int) (graphicsdriver.Image, error) {
	i := &Image{
		id:               g.genNextImageID(),
		graphics:         g,
		width:            width,
		height:           height,
		last3DClearFrame: -1,
	}
	w := graphics.InternalImageSize(width)
	h := graphics.InternalImageSize(height)
	g.checkSize(w, h)
	t, err := g.context.newTexture(w, h)
	if err != nil {
		return nil, err
	}
	i.texture = t
	g.addImage(i)
	return i, nil
}

func (g *Graphics) NewScreenFramebufferImage(width, height int) (graphicsdriver.Image, error) {
	g.checkSize(width, height)
	i := &Image{
		id:               g.genNextImageID(),
		graphics:         g,
		width:            width,
		height:           height,
		screen:           true,
		last3DClearFrame: -1,
	}
	g.addImage(i)
	return i, nil
}

func (g *Graphics) addImage(img *Image) {
	if g.images == nil {
		g.images = map[graphicsdriver.ImageID]*Image{}
	}
	if _, ok := g.images[img.id]; ok {
		panic(fmt.Sprintf("opengl: image ID %d was already registered", img.id))
	}
	g.images[img.id] = img
}

func (g *Graphics) removeImage(img *Image) {
	delete(g.images, img.id)
}

func (g *Graphics) Initialize() error {
	if err := g.makeContextCurrent(); err != nil {
		return err
	}
	if err := g.state.reset(&g.context); err != nil {
		return err
	}
	return nil
}

// Reset resets or initializes the current OpenGL state.
func (g *Graphics) Reset() error {
	return g.state.reset(&g.context)
}

func (g *Graphics) SetVertices(vertices []float32, indices []uint32) error {
	g.state.setVertices(&g.context, vertices, indices)
	return nil
}

func (g *Graphics) uniformVariableName(idx int) string {
	if v, ok := g.uniformVariableNameCache[idx]; ok {
		return v
	}
	if g.uniformVariableNameCache == nil {
		g.uniformVariableNameCache = map[int]string{}
	}
	name := fmt.Sprintf("U%d", idx)
	g.uniformVariableNameCache[idx] = name
	return name
}

func (g *Graphics) DrawTriangles(dstID graphicsdriver.ImageID, srcIDs [graphics.ShaderSrcImageCount]graphicsdriver.ImageID, shaderID graphicsdriver.ShaderID, dstRegions []graphicsdriver.DstRegion, indexOffset int, blend graphicsdriver.Blend, uniforms []uint32, fillRule graphicsdriver.FillRule) error {
	return g.DrawTrianglesWithMode(dstID, srcIDs, shaderID, dstRegions, indexOffset, blend, uniforms, fillRule, graphicsdriver.DrawModeDefault)
}

func (g *Graphics) DrawTrianglesWithMode(dstID graphicsdriver.ImageID, srcIDs [graphics.ShaderSrcImageCount]graphicsdriver.ImageID, shaderID graphicsdriver.ShaderID, dstRegions []graphicsdriver.DstRegion, indexOffset int, blend graphicsdriver.Blend, uniforms []uint32, fillRule graphicsdriver.FillRule, drawMode graphicsdriver.DrawMode) error {
	if shaderID == graphicsdriver.InvalidShaderID {
		return fmt.Errorf("opengl: shader ID is invalid")
	}

	destination := g.images[dstID]

	g.drawCalled = true

	if err := destination.setViewport(drawMode); err != nil {
		return err
	}
	g.context.blend(blend)

	shader := g.shaders[shaderID]
	program := shader.p

	ulen := len(shader.ir.Uniforms)
	if cap(g.uniformVars) < ulen {
		g.uniformVars = make([]uniformVariable, ulen)
	} else {
		g.uniformVars = g.uniformVars[:ulen]
	}

	var idx int
	for i, typ := range shader.ir.Uniforms {
		n := typ.DwordCount()
		g.uniformVars[i].name = g.uniformVariableName(i)
		g.uniformVars[i].value = uniforms[idx : idx+n]
		g.uniformVars[i].typ = typ
		idx += n
	}

	// ebiten_3d_adjust drives the vertex-shader epilogue that canonicalizes
	// 3D draws (DrawMode3D) on OpenGL: y is flipped to match the
	// cross-backend orientation contract (NDC +y = image top), and the
	// canonical clip z in [0, w] is remapped to OpenGL's [-w, w]. Regular 2D
	// draws pass 0 (identity). The two distinct backing arrays keep the
	// uniform cache's change detection working.
	adjust := adjust3DOff[:]
	if drawMode == graphicsdriver.DrawMode3D {
		adjust = adjust3DOn[:]
	}
	g.uniformVars = append(g.uniformVars, uniformVariable{
		name:  "ebiten_3d_adjust",
		value: adjust,
		typ:   shaderir.Type{Main: shaderir.Float},
	})

	// In OpenGL, the NDC's Y direction is upward, so flip the Y direction for the final framebuffer.
	if destination.screen {
		const idx = graphics.ProjectionMatrixUniformVariableIndex
		// Invert the sign bits as float32 values.
		g.uniformVars[idx].value[1] ^= 1 << 31
		g.uniformVars[idx].value[5] ^= 1 << 31
		g.uniformVars[idx].value[9] ^= 1 << 31
		g.uniformVars[idx].value[13] ^= 1 << 31
	}

	var imgs [graphics.ShaderSrcImageCount]textureVariable
	for i, srcID := range srcIDs {
		if srcID == graphicsdriver.InvalidImageID {
			continue
		}
		imgs[i].valid = true
		imgs[i].native = g.images[srcID].texture
	}

	if err := g.useProgram(program, g.uniformVars, imgs); err != nil {
		return err
	}

	for i := range g.uniformVars {
		g.uniformVars[i] = uniformVariable{}
	}
	g.uniformVars = g.uniformVars[:0]

	use3D := drawMode == graphicsdriver.DrawMode3D
	if use3D {
		if !destination.depthBufferEnabled {
			return fmt.Errorf("opengl: draw target must enable depth buffer for drawMode3D")
		}
		if err := destination.ensureDepthBuffer(); err != nil {
			return err
		}
		g.context.ctx.Enable(gl.DEPTH_TEST)
		defer g.context.ctx.Disable(gl.DEPTH_TEST)

		// Contract winding: clockwise in y-up NDC = front face (see
		// RENDER_3D.md). The 3D canonicalization epilogue flips y on
		// OpenGL, which inverts the rasterized winding, so contract-front
		// triangles arrive counterclockwise in window coordinates.
		g.context.ctx.Enable(gl.CULL_FACE)
		g.context.ctx.CullFace(gl.BACK)
		g.context.ctx.FrontFace(gl.CCW)
		defer g.context.ctx.Disable(gl.CULL_FACE)

		if destination.needs3DClear(g.frame) {
			w, h := destination.viewportSizeForMode(drawMode)
			g.context.ctx.Scissor(0, 0, int32(w), int32(h))
			g.context.ctx.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
		}
	}

	if fillRule != graphicsdriver.FillRuleFillAll {
		if err := destination.ensureStencilBuffer(); err != nil {
			return err
		}
		g.context.ctx.Enable(gl.STENCIL_TEST)
	}

	for _, dstRegion := range dstRegions {
		g.context.ctx.Scissor(
			int32(dstRegion.Region.Min.X),
			int32(dstRegion.Region.Min.Y),
			int32(dstRegion.Region.Dx()),
			int32(dstRegion.Region.Dy()),
		)
		switch fillRule {
		case graphicsdriver.FillRuleNonZero:
			g.context.ctx.Clear(gl.STENCIL_BUFFER_BIT)
			g.context.ctx.StencilFunc(gl.ALWAYS, 0x00, 0xff)
			g.context.ctx.StencilOpSeparate(gl.FRONT, gl.KEEP, gl.KEEP, gl.INCR_WRAP)
			g.context.ctx.StencilOpSeparate(gl.BACK, gl.KEEP, gl.KEEP, gl.DECR_WRAP)
			g.context.ctx.ColorMask(false, false, false, false)
			g.context.ctx.DrawElements(gl.TRIANGLES, int32(dstRegion.IndexCount), gl.UNSIGNED_INT, indexOffset*int(unsafe.Sizeof(uint32(0))))
		case graphicsdriver.FillRuleEvenOdd:
			g.context.ctx.Clear(gl.STENCIL_BUFFER_BIT)
			g.context.ctx.StencilFunc(gl.ALWAYS, 0x00, 0xff)
			g.context.ctx.StencilOpSeparate(gl.FRONT_AND_BACK, gl.KEEP, gl.KEEP, gl.INVERT)
			g.context.ctx.ColorMask(false, false, false, false)

			g.context.ctx.DrawElements(gl.TRIANGLES, int32(dstRegion.IndexCount), gl.UNSIGNED_INT, indexOffset*int(unsafe.Sizeof(uint32(0))))
		}
		if fillRule != graphicsdriver.FillRuleFillAll {
			g.context.ctx.StencilFunc(gl.NOTEQUAL, 0x00, 0xff)
			g.context.ctx.StencilOpSeparate(gl.FRONT_AND_BACK, gl.KEEP, gl.KEEP, gl.KEEP)
			g.context.ctx.ColorMask(true, true, true, true)
		}
		g.context.ctx.DrawElements(gl.TRIANGLES, int32(dstRegion.IndexCount), gl.UNSIGNED_INT, indexOffset*int(unsafe.Sizeof(uint32(0))))
		indexOffset += dstRegion.IndexCount
	}

	if fillRule != graphicsdriver.FillRuleFillAll {
		g.context.ctx.Disable(gl.STENCIL_TEST)
	}

	return nil
}

func (g *Graphics) SetVsyncEnabled(enabled bool) {
	g.vsync = enabled
}

func (g *Graphics) NeedsClearingScreen() bool {
	return true
}

func (g *Graphics) MaxImageSize() int {
	return g.context.getMaxTextureSize()
}

func (g *Graphics) EnsureDepthForImage(imgID graphicsdriver.ImageID, width, height int) error {
	img, ok := g.images[imgID]
	if !ok {
		return fmt.Errorf("opengl: image ID %d was not found when ensuring depth", imgID)
	}
	if width <= 0 || height <= 0 {
		width, height = img.viewportSize()
	}
	img.depthBufferEnabled = true
	return img.ensureDepthBufferSize(width, height)
}

func (g *Graphics) NewShader(program *shaderir.Program) (graphicsdriver.Shader, error) {
	s, err := newShader(g.genNextShaderID(), g, program)
	if err != nil {
		return nil, err
	}
	g.addShader(s)
	return s, nil
}

func (g *Graphics) addShader(shader *Shader) {
	if g.shaders == nil {
		g.shaders = map[graphicsdriver.ShaderID]*Shader{}
	}
	if _, ok := g.shaders[shader.id]; ok {
		panic(fmt.Sprintf("opengl: shader ID %d was already registered", shader.id))
	}
	g.shaders[shader.id] = shader
}

func (g *Graphics) removeShader(shader *Shader) {
	delete(g.shaders, shader.id)
}
