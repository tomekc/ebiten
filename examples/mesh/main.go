package main

import (
	"bytes"
	"fmt"
	"image/color"
	"log"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	res "github.com/hajimehoshi/ebiten/v2/examples/resources/images/shader"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type renderMode int

const (
	renderModeTextured renderMode = iota
	renderModeWireframe
)

func (m renderMode) String() string {
	switch m {
	case renderModeWireframe:
		return "wireframe"
	default:
		return "textured"
	}
}

type Game struct {
	vertices []ebiten.Vertex
	indices  []uint16
	angle    float32
	texture  *ebiten.Image
	renderer *ebiten.Renderer3D
	mode     renderMode
}

func NewGame() (*Game, error) {
	tex, err := loadTexture()
	if err != nil {
		return nil, err
	}

	tw, th := tex.Bounds().Dx(), tex.Bounds().Dy()
	vertices, indices := buildCube(float32(tw), float32(th))

	renderer3D := ebiten.NewRenderer3D()

	return &Game{
		vertices: vertices,
		indices:  indices,
		texture:  tex,
		renderer: renderer3D,
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
				ColorA:  1.0,
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
	if inpututil.IsKeyJustPressed(ebiten.Key1) {
		g.mode = renderModeTextured
	}
	if inpututil.IsKeyJustPressed(ebiten.Key2) {
		g.mode = renderModeWireframe
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	switch g.mode {
	case renderModeWireframe:
		g.DrawWireframe(screen)
	default:
		g.renderer.Begin(screen)
		g.Draw3DMesh()
		g.renderer.End(screen)
	}

	ebitenutil.DebugPrint(screen, fmt.Sprintf("Mesh example: rotating cube\n1: textured  2: wireframe\nMode: %s", g.mode))
	di := ebiten.DebugInfo{}
	ebiten.ReadDebugInfo(&di)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Graphics library: %v", di.GraphicsLibrary), 0, 60)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("TPS: %0.2f\nFPS: %0.2f", ebiten.ActualTPS(), ebiten.ActualFPS()), 0, 80)
}

func (g *Game) Draw3DMesh() {
	aspect := g.renderer.Aspect()

	proj := perspective(float32(math.Pi)/3, aspect, 0.1, 10)
	view := lookAt(vec3{0, 0, 5}, vec3{0, 0, 0}, vec3{0, 1, 0})
	rotY := rotate(g.angle, vec3{0, 1, 0.5})
	model := rotY
	mvp := mulMat4(proj, mulMat4(view, model))

	light := normalize(vec3{0, 0, 1})

	uniforms := map[string]interface{}{
		"MVP":      mvp.toSlice(),
		"NormalM":  rotY.toSlice(),
		"LightDir": []float32{light.x, light.y, light.z},
	}

	opts3d := &ebiten.DrawTriangles3DOptions{
		Uniforms: uniforms,
		Images:   [4]*ebiten.Image{g.texture},
	}
	g.renderer.DrawTriangles3D(g.vertices, g.indices, opts3d)
}

func (g *Game) DrawWireframe(screen *ebiten.Image) {
	w, h := float32(screen.Bounds().Dx()), float32(screen.Bounds().Dy())
	aspect := w / h

	proj := perspective(float32(math.Pi)/3, aspect, 0.1, 10)
	view := lookAt(vec3{0, 0, 5}, vec3{0, 0, 0}, vec3{0, 1, 0})
	model := rotate(g.angle, vec3{0, 1, 0.5})
	mvp := mulMat4(proj, mulMat4(view, model))

	corners := []vec3{
		{-1, -1, -1}, {1, -1, -1}, {1, 1, -1}, {-1, 1, -1},
		{-1, -1, 1}, {1, -1, 1}, {1, 1, 1}, {-1, 1, 1},
	}
	edges := [][2]int{
		{0, 1}, {1, 2}, {2, 3}, {3, 0},
		{4, 5}, {5, 6}, {6, 7}, {7, 4},
		{0, 4}, {1, 5}, {2, 6}, {3, 7},
	}

	project := func(p vec3) (float32, float32, bool) {
		clip := mvp.mulVec4(vec4{p.x, p.y, p.z, 1})
		if clip.w <= 0 {
			return 0, 0, false
		}
		ndcX := clip.x / clip.w
		ndcY := clip.y / clip.w
		return (ndcX + 1) * 0.5 * w, (1 - ndcY) * 0.5 * h, true
	}

	for _, e := range edges {
		x0, y0, ok0 := project(corners[e[0]])
		x1, y1, ok1 := project(corners[e[1]])
		if ok0 && ok1 {
			vector.StrokeLine(screen, x0, y0, x1, y1, 2, color.White, true)
		}
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

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
