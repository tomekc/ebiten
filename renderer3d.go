package ebiten

import (
	"image"
)

const shaderSrc = `
package main

var MVP mat4
var NormalM mat4
var LightDir vec3

func Vertex(dstPos vec2, srcPos vec2, color vec4, custom vec4) (vec4, vec2, vec4, vec4) {
	pos := vec3(custom.x, custom.y, custom.z)
	clip := MVP * vec4(pos, 1)
	normal := normalize((NormalM * vec4(color.xyz, 0)).xyz)
	return clip, srcPos, vec4(normal, 1), custom
}

func Fragment(dstPos vec4, srcPos vec2, normal vec4, custom vec4) vec4 {
	n := normalize(normal.xyz)
	l := normalize(LightDir)
	diff := max(dot(n, l), 0)
	texel := imageSrc0At(srcPos)
	base := texel.rgb
	shaded := base * (0.2 + 0.8*diff)
	return vec4(shaded, texel.a)
}
`

// Renderer3D manages an offscreen render target with a dedicated depth buffer.
// Use Begin to obtain the render target, draw 3D content into it, then call End
// to composite the result back to another image (typically the screen).
type Renderer3D struct {
	target               *Image
	size                 image.Point
	shader               *Shader
	OffScreenBlitOptions *DrawImageOptions
}

// NewRenderer3D creates a new Renderer3D with the given size.
func NewRenderer3D() *Renderer3D {
	shader, err := NewShader([]byte(shaderSrc))
	if err != nil {
		panic(err)
	}
	return &Renderer3D{
		shader: shader,
	}
}

// Resize recreates the underlying render target when the size changes.
func (r *Renderer3D) Resize(width, height int) {
	r.resize(width, height)
}

func (r *Renderer3D) resize(width, height int) {
	if width <= 0 || height <= 0 {
		panic("ebiten: Renderer3D size must be positive")
	}

	if r.size.X != width || r.size.Y != height {
		r.size = image.Pt(width, height)
		if r.target != nil {
			r.target.Dispose()
			r.target = nil
		}

	}

}

// Begin returns the underlying render target. Callers should draw their 3D content
// into the returned image before invoking End.
func (r *Renderer3D) Begin(screen *Image) *Image {
	r.resize(screen.Bounds().Dx(), screen.Bounds().Dy())
	r.ensureTarget()
	return r.target
}

func (r *Renderer3D) ensureTarget() {
	if r.target == nil {
		img := NewImageWithOptions(image.Rect(0, 0, r.size.X, r.size.Y), &NewImageOptions{Unmanaged: true})
		img.EnableDepthBuffer()
		r.target = img
	}
}

// End composites the render target onto dst. opts can be nil for the default draw.
func (r *Renderer3D) End(dst *Image) {
	if dst == nil {
		panic("ebiten: Renderer3D.End destination image must not be nil")
	}
	dst.DrawImage(r.target, r.OffScreenBlitOptions)
}

// Target returns the underlying offscreen image so callers can issue custom draws
// or re-use it between Begin/End calls.
func (r *Renderer3D) Target() *Image {
	return r.target
}

// DrawTrianglesShader is a convenience wrapper that forwards to the underlying render target.
func (r *Renderer3D) DrawTrianglesShader(vertices []Vertex, indices []uint16, shader *Shader, opts *DrawTrianglesShaderOptions) {
	r.target.DrawTrianglesShader(vertices, indices, shader, opts)
}

type DrawTriangles3DOptions struct {
	Uniforms map[string]any
	Images   [4]*Image
}

func (r *Renderer3D) DrawTriangles3D(vertices []Vertex, indices []uint16, opts3D *DrawTriangles3DOptions) {
	opts := &DrawTrianglesShaderOptions{
		RawDstCoordinates: true,
		Images:            opts3D.Images,
		Uniforms:          opts3D.Uniforms,
	}
	r.target.DrawTrianglesShader(vertices, indices, r.shader, opts)
}

// Dispose releases resources held by the renderer.
func (r *Renderer3D) Dispose() {
	if r.target != nil {
		r.target.Dispose()
		r.target = nil
	}
}

func (r *Renderer3D) Aspect() float32 {
	return float32(r.size.X) / float32(r.size.Y)
}
