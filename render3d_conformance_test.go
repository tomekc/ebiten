// Copyright 2026 The Ebitengine Authors
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

package ebiten_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// These tests define the cross-backend contract of the experimental 3D mode
// (EnableDepthBuffer + DrawMode3D). Every backend must behave identically:
//
//   - Clip space: x right, y up, z in [0, w] (post-divide NDC z in [0, 1]).
//   - Orientation: NDC +y is the TOP of the destination image.
//   - Depth: LessEqual compare, depth writes on, cleared to 1 once per frame.
//   - Winding: clockwise as seen in NDC (y up) = front face; back faces are
//     culled. (In destination-image space, y down, front faces appear
//     counterclockwise. Settled empirically on Metal - see conf3DTri.)
//   - Draws into a 3D target survive interleaved draws to other images
//     within the same frame.
//   - 2D rendering (including stencil-based vector fills) is unaffected.
//
// See RENDER_3D.md.

const conf3DShaderSrc = `
package main

func Vertex(dstPos vec2, srcPos vec2, color vec4, custom vec4) (vec4, vec2, vec4, vec4) {
	return custom, srcPos, color, custom
}

func Fragment(dstPos vec4, srcPos vec2, color vec4, custom vec4) vec4 {
	return color
}
`

func conf3DGraphicsLibrary() ebiten.GraphicsLibrary {
	var d ebiten.DebugInfo
	ebiten.ReadDebugInfo(&d)
	return d.GraphicsLibrary
}

func newConf3DTarget(w, h int) *ebiten.Image {
	img := ebiten.NewImageWithOptions(image.Rect(0, 0, w, h), &ebiten.NewImageOptions{Unmanaged: true})
	img.EnableDepthBuffer()
	return img
}

func conf3DShader(t *testing.T) *ebiten.Shader {
	t.Helper()
	s, err := ebiten.NewShader([]byte(conf3DShaderSrc))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// conf3DTri builds one solid-color triangle in canonical clip space
// (z in [0, w], w = 1). The emitted vertex order makes the triangle a FRONT
// face: clockwise as seen in NDC (y up). This was settled empirically on
// Metal (the original y-down-clockwise guess rendered nothing - every
// triangle was culled). Every test shares this helper, so the convention
// lives in exactly one place.
func conf3DTri(x0, y0, x1, y1, x2, y2, z float32, r, g, b float32) ([]ebiten.Vertex, []uint16) {
	mk := func(x, y float32) ebiten.Vertex {
		return ebiten.Vertex{
			ColorR: r, ColorG: g, ColorB: b, ColorA: 1,
			Custom0: x, Custom1: y, Custom2: z, Custom3: 1,
		}
	}
	return []ebiten.Vertex{mk(x0, y0), mk(x2, y2), mk(x1, y1)}, []uint16{0, 1, 2}
}

func conf3DDraw(dst *ebiten.Image, s *ebiten.Shader, vs []ebiten.Vertex, is []uint16) {
	dst.DrawTrianglesShader(vs, is, s, &ebiten.DrawTrianglesShaderOptions{RawDstCoordinates: true})
}

func conf3DPixelAt(t *testing.T, img *ebiten.Image, x, y int) color.RGBA {
	t.Helper()
	c, _ := img.At(x, y).(color.RGBA)
	return c
}

// TestRender3D_DepthOrdering: a near triangle drawn AFTER a far one must
// still win (LessEqual + depth write), and a far triangle drawn after the
// near one must lose.
func TestRender3D_DepthOrdering(t *testing.T) {
	s := conf3DShader(t)
	dst := newConf3DTarget(64, 64)

	vs, is := conf3DTri(-1, -1, 3, -1, -1, 3, 0.9, 1, 0, 0) // far, red
	conf3DDraw(dst, s, vs, is)
	vs, is = conf3DTri(-1, -1, 3, -1, -1, 3, 0.1, 0, 1, 0) // near, green
	conf3DDraw(dst, s, vs, is)
	vs, is = conf3DTri(-1, -1, 3, -1, -1, 3, 0.9, 0, 0, 1) // far again, blue
	conf3DDraw(dst, s, vs, is)

	if px := conf3DPixelAt(t, dst, 32, 32); px.G < 200 || px.R > 50 || px.B > 50 {
		t.Fatalf("depth test failed: center pixel %v, want green", px)
	}
}

// TestRender3D_Orientation: a triangle covering the NDC top-left quadrant
// must land at the TOP-left of the destination image. The assertion defines
// the contract: NDC +y = image top.
func TestRender3D_Orientation(t *testing.T) {
	s := conf3DShader(t)
	dst := newConf3DTarget(64, 64)

	// Right triangle over NDC x in [-1,0], y in [0,1].
	vs, is := conf3DTri(-1, 0, 0, 0, -1, 1, 0.5, 1, 1, 1)
	conf3DDraw(dst, s, vs, is)

	// Interior point NDC(-0.75, 0.25) -> pixel (8, 24).
	if px := conf3DPixelAt(t, dst, 8, 24); px.R < 200 {
		t.Fatalf("top-left NDC quadrant not at image top-left: pixel(8,24)=%v", px)
	}
	// Vertically mirrored probe: must be empty (catches y-flipped backends).
	if px := conf3DPixelAt(t, dst, 8, 40); px.A != 0 {
		t.Fatalf("y-flip detected: pixel(8,40)=%v, want empty", px)
	}
	// Bottom-right quadrant: empty.
	if px := conf3DPixelAt(t, dst, 56, 40); px.A != 0 {
		t.Fatalf("unexpected coverage at pixel(56,40)=%v", px)
	}
}

// TestRender3D_ZRange: the canonical clip volume is z in [0, w].
// In-range geometry (NDC z 0.01 and 0.99) must be visible, and
// out-of-contract geometry (NDC z < 0) must be clipped on EVERY backend -
// backends whose native NDC allows z in [-1, 1] (OpenGL) must remap so that
// behavior matches Metal/DirectX.
//
// NOTE: until the y-flip divergence is fixed (task A2), the z<0 assertion is
// vacuous on OpenGL - the out-of-contract triangle renders y-flipped away
// from its probe pixel. A2's verification explicitly re-arms it.
func TestRender3D_ZRange(t *testing.T) {
	s := conf3DShader(t)
	dst := newConf3DTarget(64, 64)

	vs, is := conf3DTri(-1, -1, 0, -1, -1, 3, 0.01, 1, 0, 0) // near z, left region
	conf3DDraw(dst, s, vs, is)
	vs, is = conf3DTri(1, 1, 0, 1, 1, -3, 0.99, 0, 1, 0) // far z, right region
	conf3DDraw(dst, s, vs, is)
	// Out of contract: z = -0.5 (valid only in GL-style clip space).
	vs, is = conf3DTri(-0.3, -0.9, 0.3, -0.9, 0, -0.5, -0.5, 0, 0, 1) // bottom center, blue
	conf3DDraw(dst, s, vs, is)

	if px := conf3DPixelAt(t, dst, 4, 32); px.R < 200 {
		t.Fatalf("near-z (NDC 0.01) geometry clipped: %v", px)
	}
	if px := conf3DPixelAt(t, dst, 60, 32); px.G < 200 {
		t.Fatalf("far-z (NDC 0.99) geometry clipped: %v", px)
	}
	if px := conf3DPixelAt(t, dst, 32, 56); px.B > 50 {
		t.Fatalf("out-of-contract z=-0.5 geometry was drawn (pixel(32,56)=%v); canonical clip volume is z in [0,w]", px)
	}
}

// TestRender3D_Winding: triangles wound clockwise in NDC (y up) are front
// faces and render; the reverse winding is a back face and is culled.
func TestRender3D_Winding(t *testing.T) {
	if lib := conf3DGraphicsLibrary(); lib == ebiten.GraphicsLibraryOpenGL {
		t.Skip("OpenGL 3D path does not cull yet (fork task A3)")
	}
	s := conf3DShader(t)
	dst := newConf3DTarget(64, 64)

	// Front (helper order = CW in y-up NDC): left region.
	vs, is := conf3DTri(-0.9, -0.5, -0.1, -0.5, -0.5, 0.5, 0.5, 0, 1, 0)
	conf3DDraw(dst, s, vs, is)
	// Back (reversed order of the same shape, shifted right): must be culled.
	vs, is = conf3DTri(0.1, -0.5, 0.9, -0.5, 0.5, 0.5, 0.5, 1, 0, 0)
	vs[1], vs[2] = vs[2], vs[1]
	conf3DDraw(dst, s, vs, is)

	// Interior of the front triangle: NDC(-0.5, -0.1) -> pixel (16, 35).
	if px := conf3DPixelAt(t, dst, 16, 35); px.G < 200 {
		t.Fatalf("front-wound (CW) triangle was culled: pixel(16,35)=%v", px)
	}
	// Interior of the back triangle: NDC(0.5, -0.1) -> pixel (48, 35).
	if px := conf3DPixelAt(t, dst, 48, 35); px.R > 50 {
		t.Fatalf("back-wound (CCW) triangle was NOT culled: pixel(48,35)=%v", px)
	}
}

// TestRender3D_InterleavedDraws: draws into a 3D target must survive an
// interleaved draw to a different image within the same frame. (Backends
// must clear a 3D target once per frame, not once per render pass.)
func TestRender3D_InterleavedDraws(t *testing.T) {
	if lib := conf3DGraphicsLibrary(); lib == ebiten.GraphicsLibraryMetal {
		t.Skip("Metal clears 3D targets per render pass, wiping earlier draws (fork task A4)")
	}
	s := conf3DShader(t)
	dst := newConf3DTarget(64, 64)
	other := ebiten.NewImage(16, 16)

	// Left triangle into the 3D target.
	vs, is := conf3DTri(-0.9, -0.5, -0.1, -0.5, -0.5, 0.5, 0.5, 1, 0, 0)
	conf3DDraw(dst, s, vs, is)
	// Interleaved 2D draw to another image (splits the render pass).
	other.Fill(color.RGBA{R: 255, A: 255})
	// Right triangle into the 3D target.
	vs, is = conf3DTri(0.1, -0.5, 0.9, -0.5, 0.5, 0.5, 0.5, 0, 1, 0)
	conf3DDraw(dst, s, vs, is)

	if px := conf3DPixelAt(t, dst, 16, 35); px.R < 200 {
		t.Fatalf("first 3D draw was wiped by an interleaved draw: pixel(16,35)=%v", px)
	}
	if px := conf3DPixelAt(t, dst, 48, 35); px.G < 200 {
		t.Fatalf("second 3D draw missing: pixel(48,35)=%v", px)
	}
}

// TestRender3D_2DIsolation: stencil-based vector rendering on a normal image
// must be unaffected by 3D draws earlier in the same frame.
func TestRender3D_2DIsolation(t *testing.T) {
	s := conf3DShader(t)
	dst := newConf3DTarget(64, 64)

	vs, is := conf3DTri(-1, -1, 3, -1, -1, 3, 0.5, 1, 0, 0)
	conf3DDraw(dst, s, vs, is)

	plain := ebiten.NewImage(64, 64)
	// Anti-aliased circle fill exercises the stencil path.
	vector.FillCircle(plain, 32, 32, 20, color.RGBA{G: 255, A: 255}, true)
	vector.StrokeLine(plain, 0, 0, 63, 63, 3, color.RGBA{B: 255, A: 255}, false)

	if px := conf3DPixelAt(t, plain, 32, 20); px.G < 200 {
		t.Fatalf("vector circle fill broken after 3D draws: %v", px)
	}
	if px := conf3DPixelAt(t, plain, 8, 8); px.B < 200 {
		t.Fatalf("vector stroke broken after 3D draws: %v", px)
	}
}
