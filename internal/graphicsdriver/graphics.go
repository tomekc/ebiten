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

package graphicsdriver

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2/internal/graphics"
	"github.com/hajimehoshi/ebiten/v2/internal/shaderir"
)

type DstRegion struct {
	Region     image.Rectangle
	IndexCount int
}

type FillRule int

const (
	FillRuleFillAll FillRule = iota
	FillRuleNonZero
	FillRuleEvenOdd
)

func (f FillRule) String() string {
	switch f {
	case FillRuleFillAll:
		return "FillRuleFillAll"
	case FillRuleNonZero:
		return "FillRuleNonZero"
	case FillRuleEvenOdd:
		return "FillRuleEvenOdd"
	default:
		return fmt.Sprintf("FillRule(%d)", f)
	}
}

type DrawMode int

const (
	DrawModeDefault DrawMode = iota
	DrawMode3D
)

const (
	InvalidImageID  = 0
	InvalidShaderID = 0
)

type Graphics interface {
	Initialize() error
	Begin() error
	End(present bool) error
	SetTransparent(transparent bool)
	SetVertices(vertices []float32, indices []uint32) error
	NewImage(width, height int) (Image, error)
	NewScreenFramebufferImage(width, height int) (Image, error)
	SetVsyncEnabled(enabled bool)
	NeedsClearingScreen() bool
	MaxImageSize() int

	NewShader(program *shaderir.Program) (Shader, error)

	// DrawTriangles draws an image onto another image with the given parameters.
	DrawTriangles(dst ImageID, srcs [graphics.ShaderSrcImageCount]ImageID, shader ShaderID, dstRegions []DstRegion, indexOffset int, blend Blend, uniforms []uint32, fillRule FillRule) error
}

type DrawTrianglesWithMode interface {
	DrawTrianglesWithMode(dst ImageID, srcs [graphics.ShaderSrcImageCount]ImageID, shader ShaderID, dstRegions []DstRegion, indexOffset int, blend Blend, uniforms []uint32, fillRule FillRule, mode DrawMode) error
}

// DepthTextureAttacher represents a graphics driver that can attach a depth buffer
// to an existing image. This is optional and drivers that don't support depth
// simply omit implementing this interface.
type DepthTextureAttacher interface {
	// EnsureDepthForImage guarantees that a depth buffer backing exists for the
	// specified image size. Calling this multiple times must be safe.
	EnsureDepthForImage(img ImageID, width, height int) error
}

type Resetter interface {
	Reset() error
}

type Image interface {
	ID() ImageID
	Dispose()
	ReadPixels(args []PixelsArgs) error
	WritePixels(args []PixelsArgs) error
}

type ImageID int

type PixelsArgs struct {
	Pixels []byte
	Region image.Rectangle
}

type Shader interface {
	ID() ShaderID
	Dispose()
}

type ShaderID int

type ColorSpace int

const (
	ColorSpaceDefault ColorSpace = iota
	ColorSpaceSRGB
	ColorSpaceDisplayP3
)
