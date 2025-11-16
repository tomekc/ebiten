package main

import (
	"bytes"
	"image"
	"image/color"
	"log"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	res "github.com/hajimehoshi/ebiten/v2/examples/resources/images/shader"
)

type vec3 struct {
	x, y, z float32
}

type vec4 struct {
	x, y, z, w float32
}

type mat4 [16]float32

func identity() mat4 {
	var m mat4
	m[0] = 1
	m[5] = 1
	m[10] = 1
	m[15] = 1
	return m
}

func (m mat4) at(row, col int) float32 {
	return m[col*4+row]
}

func (m *mat4) set(row, col int, v float32) {
	(*m)[col*4+row] = v
}

func mulMat4(a, b mat4) mat4 {
	var r mat4
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			var s float32
			for k := 0; k < 4; k++ {
				s += a.at(row, k) * b.at(k, col)
			}
			r.set(row, col, s)
		}
	}
	return r
}

func (m mat4) mulVec4(v vec4) vec4 {
	return vec4{
		m.at(0, 0)*v.x + m.at(0, 1)*v.y + m.at(0, 2)*v.z + m.at(0, 3)*v.w,
		m.at(1, 0)*v.x + m.at(1, 1)*v.y + m.at(1, 2)*v.z + m.at(1, 3)*v.w,
		m.at(2, 0)*v.x + m.at(2, 1)*v.y + m.at(2, 2)*v.z + m.at(2, 3)*v.w,
		m.at(3, 0)*v.x + m.at(3, 1)*v.y + m.at(3, 2)*v.z + m.at(3, 3)*v.w,
	}
}

func perspective(fovY, aspect, near, far float32) mat4 {

	// fovy = (fovy * math.Pi) / 180.0 // convert from degrees to radians
	nmf, f := near-far, float32(1./math.Tan(float64(fovY)/2.0))

	return mat4{float32(f / aspect), 0, 0, 0, 0, float32(f), 0, 0, 0, 0, float32((near + far) / nmf), -1, 0, 0, float32((2. * far * near) / nmf), 0}

	//f := float32(1.0 / math.Tan(float64(fovY)/2.0))
	//var m mat4
	//m.set(0, 0, f/aspect)
	//m.set(1, 1, f)
	//m.set(2, 2, (far+near)/(near-far))
	//m.set(2, 3, (2*far*near)/(near-far))
	//m.set(3, 2, -1)
	//return m
}

func normalize(v vec3) vec3 {
	l := float32(math.Sqrt(float64(v.x*v.x + v.y*v.y + v.z*v.z)))
	if l == 0 {
		return v
	}
	return vec3{v.x / l, v.y / l, v.z / l}
}

func cross(a, b vec3) vec3 {
	return vec3{
		a.y*b.z - a.z*b.y,
		a.z*b.x - a.x*b.z,
		a.x*b.y - a.y*b.x,
	}
}

func dot(a, b vec3) float32 {
	return a.x*b.x + a.y*b.y + a.z*b.z
}

func lookAt(eye, center, up vec3) mat4 {
	f := normalize(vec3{center.x - eye.x, center.y - eye.y, center.z - eye.z})
	s := normalize(cross(f, up))
	u := cross(s, f)

	m := identity()
	m.set(0, 0, s.x)
	m.set(1, 0, s.y)
	m.set(2, 0, s.z)

	m.set(0, 1, u.x)
	m.set(1, 1, u.y)
	m.set(2, 1, u.z)

	m.set(0, 2, -f.x)
	m.set(1, 2, -f.y)
	m.set(2, 2, -f.z)

	m.set(0, 3, -dot(s, eye))
	m.set(1, 3, -dot(u, eye))
	m.set(2, 3, dot(f, eye))
	return m
}

func rotate(angle float32, axis vec3) mat4 {
	a := normalize(axis)
	s := float32(math.Sin(float64(angle)))
	c := float32(math.Cos(float64(angle)))
	oc := 1 - c

	var m mat4
	m.set(0, 0, oc*a.x*a.x+c)
	m.set(0, 1, oc*a.x*a.y-a.z*s)
	m.set(0, 2, oc*a.x*a.z+a.y*s)

	m.set(1, 0, oc*a.x*a.y+a.z*s)
	m.set(1, 1, oc*a.y*a.y+c)
	m.set(1, 2, oc*a.y*a.z-a.x*s)

	m.set(2, 0, oc*a.x*a.z-a.y*s)
	m.set(2, 1, oc*a.y*a.z+a.x*s)
	m.set(2, 2, oc*a.z*a.z+c)

	m.set(3, 3, 1)
	return m
}

func (m mat4) toSlice() []float32 {
	out := make([]float32, 16)
	copy(out, m[:])
	return out
}

func (m mat4) transpose() mat4 {
	var t mat4
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			t.set(row, col, m.at(col, row))
		}
	}
	return t
}

type Game struct {
	shader   *ebiten.Shader
	vertices []ebiten.Vertex
	indices  []uint16
	angle    float32
	texture  *ebiten.Image
	rt       *ebiten.Image
}

func NewGame() (*Game, error) {
	shader, err := ebiten.NewShader([]byte(shaderSrc))
	if err != nil {
		return nil, err
	}

	tex, err := loadTexture()
	if err != nil {
		return nil, err
	}

	tw, th := tex.Bounds().Dx(), tex.Bounds().Dy()
	vertices, indices := buildCube(float32(tw), float32(th))

	return &Game{
		shader:   shader,
		vertices: vertices,
		indices:  indices,
		texture:  tex,
	}, nil
}

func loadTexture() (*ebiten.Image, error) {
	img, _, err := ebitenutil.NewImageFromReader(bytes.NewReader(res.GopherBg_png))
	if err != nil {
		return nil, err
	}
	return img, nil
}

func buildCube(texWidth, texHeight float32) ([]ebiten.Vertex, []uint16) {
	type face struct {
		normal vec3
		verts  [4]vec3
		uvs    [4][2]float32
	}

	faces := []face{
		{normal: vec3{0, 0, 1}, verts: [4]vec3{{-1, -1, 1}, {1, -1, 1}, {1, 1, 1}, {-1, 1, 1}}, uvs: [4][2]float32{{0, 1}, {1, 1}, {1, 0}, {0, 0}}},
		{normal: vec3{0, 0, -1}, verts: [4]vec3{{1, -1, -1}, {-1, -1, -1}, {-1, 1, -1}, {1, 1, -1}}, uvs: [4][2]float32{{0, 1}, {1, 1}, {1, 0}, {0, 0}}},
		{normal: vec3{0, 1, 0}, verts: [4]vec3{{-1, 1, 1}, {1, 1, 1}, {1, 1, -1}, {-1, 1, -1}}, uvs: [4][2]float32{{0, 1}, {1, 1}, {1, 0}, {0, 0}}},
		{normal: vec3{0, -1, 0}, verts: [4]vec3{{-1, -1, -1}, {1, -1, -1}, {1, -1, 1}, {-1, -1, 1}}, uvs: [4][2]float32{{0, 1}, {1, 1}, {1, 0}, {0, 0}}},
		{normal: vec3{1, 0, 0}, verts: [4]vec3{{1, -1, 1}, {1, -1, -1}, {1, 1, -1}, {1, 1, 1}}, uvs: [4][2]float32{{0, 1}, {1, 1}, {1, 0}, {0, 0}}},
		{normal: vec3{-1, 0, 0}, verts: [4]vec3{{-1, -1, -1}, {-1, -1, 1}, {-1, 1, 1}, {-1, 1, -1}}, uvs: [4][2]float32{{0, 1}, {1, 1}, {1, 0}, {0, 0}}},
	}

	var vertices []ebiten.Vertex
	for _, f := range faces {
		normal := normalize(f.normal)
		quad := f.verts
		triOrder := [6]int{0, 2, 1, 0, 3, 2}
		for _, idx := range triOrder {
			p := quad[idx]
			uv := f.uvs[idx]
			uCoord := uv[0] * texWidth
			vCoord := uv[1] * texHeight
			vertex := ebiten.Vertex{
				DstX:    0,
				DstY:    0,
				SrcX:    uCoord,
				SrcY:    vCoord,
				ColorR:  normal.x,
				ColorG:  normal.y,
				ColorB:  normal.z,
				ColorA:  1,
				Custom0: p.x,
				Custom1: p.y,
				Custom2: p.z,
				Custom3: 1,
			}
			vertices = append(vertices, vertex)
		}
	}

	indices := make([]uint16, len(vertices))
	for i := range indices {
		indices[i] = uint16(i)
	}

	return vertices, indices
}

func (g *Game) Update() error {
	g.angle += 0.02
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.NRGBA{R: 62, G: 18, B: 31, A: 255})

	width, height := screen.Size()
	rt := g.ensureRenderTarget(width, height)
	g.Draw3DMesh(rt)
	screen.DrawImage(rt, nil)

	ebitenutil.DebugPrint(screen, "Mesh example: rotating flat-shaded cube")
}

func (g *Game) ensureRenderTarget(width, height int) *ebiten.Image {
	if g.rt != nil {
		w, h := g.rt.Size()
		if w == width && h == height {
			return g.rt
		}
		g.rt.Dispose()
	}
	rt := ebiten.NewImageWithOptions(image.Rect(0, 0, width, height), &ebiten.NewImageOptions{Unmanaged: true})
	rt.EnableDepthBuffer()
	g.rt = rt
	return g.rt
}

func (g *Game) Draw3DMesh(target *ebiten.Image) {
	width, height := target.Size()
	aspect := float32(width) / float32(height)

	proj := perspective(float32(math.Pi)/3, aspect, 0.1, 10)
	view := lookAt(vec3{0, 0, 10}, vec3{0, 0, 0}, vec3{0, 1, 0})
	rotY := rotate(g.angle, vec3{0, 1, 0.5})
	model := rotY
	//rotX := rotate(g.angle*0.5, vec3{1, 0, 0})
	//model := mulMat4(rotY, rotX)
	//model := identity()
	mvp := mulMat4(proj, mulMat4(view, model))

	light := normalize(vec3{0.5, -0.5, -0.3})

	uniforms := map[string]interface{}{
		"MVP":      mvp.toSlice(),
		"NormalM":  rotY.transpose().toSlice(),
		"LightDir": []float32{light.x, light.y, light.z},
	}

	opts := &ebiten.DrawTrianglesShaderOptions{
		Uniforms:          uniforms,
		RawDstCoordinates: true,
		Images:            [4]*ebiten.Image{g.texture},
	}

	//log.Printf("Vertices: %d Indices: %d", len(g.vertices), len(g.indices))
	//log.Printf("MVP: %v", mvp.toSlice())
	//log.Printf("Vertices: %v", g.vertices)
	//for i, vertex := range g.vertices {
	//	sceen := mvp.mulVec4(vec4{
	//		x: vertex.Custom0,
	//		y: vertex.Custom1,
	//		z: vertex.Custom2,
	//		w: vertex.Custom3,
	//	})
	//	log.Printf("Vertex %d: %v", i, sceen)
	//}

	target.DrawTrianglesShader(g.vertices, g.indices, g.shader, opts)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return 640, 480
}

const shaderSrc = `
package main

var MVP mat4
var LightDir vec3

func Vertex(dstPos vec2, srcPos vec2, color vec4, custom vec4) (vec4, vec2, vec4, vec4) {
    pos := vec3(custom.x, custom.y, custom.z)
    clip := MVP * vec4(pos, 1)
    // Pass the normal through the color channel.
    return clip, srcPos, color, custom
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

func main() {
	game, err := NewGame()
	if err != nil {
		log.Fatalf("failed to initialize game: %v", err)
	}
	ebiten.SetWindowSize(960, 720)
	ebiten.SetWindowTitle("Mesh – 3D Cube Prototype")
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
