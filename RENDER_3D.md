# 3D Rendering

This fork adds an experimental hardware-depth 3D path on top of Ebitengine's 2D renderer. The key idea is to render 3D content into an offscreen `*ebiten.Image` that owns a depth buffer, then composite that color image back into the normal 2D frame.

The implementation is intentionally small: 3D is selected by the destination image, not by a global renderer mode. An image becomes a 3D render target when `EnableDepthBuffer` is called on it.

## Basic Usage

Use `Renderer3D` when you want the built-in prototype path:

```go
type Game struct {
	renderer *ebiten.Renderer3D
	texture  *ebiten.Image
	vertices []ebiten.Vertex
	indices  []uint16
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.renderer.Begin(screen)
	g.drawWorld3D()
	g.renderer.End(screen)

	// Draw normal 2D UI after the 3D color target is composited.
}
```

`Begin(screen)` resizes the offscreen target to the screen size when needed and returns the 3D target. `End(screen)` draws that target onto `screen` with `DrawImage`. Set `Renderer3D.OffScreenBlitOptions` if the composite step needs a custom blend, transform, or color scale.

The built-in `DrawTriangles3D` helper uses the internal shader in `renderer3d.go`:

```go
uniforms := map[string]any{
	"MVP":      mvp.toSlice(),
	"NormalM":  normalM.toSlice(),
	"LightDir": []float32{0, 0, 1},
}

opts := &ebiten.DrawTriangles3DOptions{
	Uniforms: uniforms,
	Images:   [4]*ebiten.Image{texture},
}
renderer.DrawTriangles3D(vertices, indices, opts)
```

You can also draw directly into `renderer.Target()` or use `Renderer3D.DrawTrianglesShader` with your own shader. The target is an unmanaged image with a depth buffer attached.

## Vertex Layout

`DrawTriangles3D` uses the existing `ebiten.Vertex` fields as a compact 3D vertex format:

* `Custom0`, `Custom1`, `Custom2`: object-space position `(x, y, z)`.
* `Custom3`: currently unused by the built-in shader, but set it to `1` for consistency.
* `ColorR`, `ColorG`, `ColorB`: object-space normal `(x, y, z)`.
* `ColorA`: vertex alpha. The built-in shader multiplies the sampled texel alpha by this value and also scales RGB to keep the result premultiplied.
* `SrcX`, `SrcY`: texture coordinates in Ebitengine texel units.
* `DstX`, `DstY`: ignored by the built-in 3D shader; examples set them to `0`.

For custom shaders, `DrawTrianglesShader` preserves `Color*` and `Custom*` as arbitrary floating-point values. Unlike plain `DrawTriangles`, these fields are not converted into premultiplied color when using the shader path.

## Shader Uniforms

The built-in shader expects these uniforms:

* `MVP mat4`: model-view-projection matrix. The vertex shader multiplies `MVP * vec4(position, 1)` and returns clip-space coordinates.
* `NormalM mat4`: transforms object-space normals into the same space as `LightDir`. For a pure rotation model matrix, this is the model rotation. For non-uniform scaling, use the inverse transpose of the model matrix's linear 3x3 part, represented as a `mat4`.
* `LightDir vec3`: vector from the shaded surface toward the light source. This is not the direction light rays travel. With the default camera, `vec3(0, 0, 1)` lights camera-facing surfaces.

The shader samples `Images[0]` and applies simple diffuse lighting:

```text
texel.rgb * (0.2 + 0.8 * max(dot(normal, LightDir), 0))
```

## Coordinate Convention

The prototype 3D path uses a right-handed world/view convention, matching the math in `examples/mesh`:

* `+X` points to the observer's right.
* `+Y` points up.
* `+Z` points toward the observer / camera.
* `-Z` points away from the observer, into the scene.

The default mesh camera is placed at `(0, 0, 5)` and looks at the origin, so it looks down the `-Z` axis. A cube face with normal `(0, 0, 1)` points toward that camera.

Matrices in `examples/mesh/math3d.go` are column-major. Matrix multiplication is written for column vectors, so transforms compose as `projection * view * model`.

### Canonical clip-space contract (all backends)

Vertex shaders for 3D draws must emit clip-space positions with:

* `x` right, `y` up: post-divide NDC `(+1, +1)` is the TOP-right of the
  destination image on every backend.
* `z` in `[0, w]`: near maps to NDC z `0`, far to `1` (Direct3D/Metal style).
  Geometry with clip z outside `[0, w]` is clipped on every backend —
  including OpenGL, whose native `[-w, +w]` volume is remapped internally.
* Winding: triangles that wind clockwise as seen in y-up NDC are front
  faces; back faces are culled (Metal/DirectX today; OpenGL after the
  culling-parity task).

Backends adapt internally so callers never branch per platform. On
OpenGL/WebGL the shader compiler wraps the user vertex entry point and
applies a driver-controlled epilogue (`ebiten_3d_adjust`) that flips y and
remaps z for `DrawMode3D` draws only; 2D draws are untouched. Use a
projection matrix that emits z in `[0, w]`, like `examples/mesh`'s
`perspective` (`zz = far/(near-far)`, `zw = far*near/(near-far)`).

`render3d_conformance_test.go` is the executable form of this contract; run
it per backend with `go test -run TestRender3D .` and
`EBITENGINE_GRAPHICS_LIBRARY=opengl go test -run TestRender3D .`.

## How 3D Is Hooked Into Ebitengine

The public API entry point is:

```go
img := ebiten.NewImageWithOptions(rect, &ebiten.NewImageOptions{Unmanaged: true})
img.EnableDepthBuffer()
```

Important constraints:

* Call `EnableDepthBuffer` only on non-sub-images.
* Use an unmanaged image for 3D render targets so the image does not live in the shared atlas.
* Depth support is backend-dependent; unsupported drivers can ignore the attach request.

Internally the path is:

1. `Image.EnableDepthBuffer` calls `ui.Image.EnableDepthBuffer`.
2. `internal/graphicscommand.Image.EnableDepthBuffer` enqueues `enableDepthBufferCommand` and marks the command image as `depthBufferEnabled`.
3. A depth-enabled destination image reports `graphicsdriver.DrawMode3D` from `drawMode()`.
4. Draw commands carry that draw mode into `drawTrianglesCommand`.
5. If the backend implements `graphicsdriver.DrawTrianglesWithMode`, the command queue calls `DrawTrianglesWithMode(..., DrawMode3D)`.
6. If the backend implements `graphicsdriver.DepthTextureAttacher`, `enableDepthBufferCommand` calls `EnsureDepthForImage`.

This keeps ordinary 2D draws on `DrawModeDefault` while draws into the 3D target get backend-specific depth state.

## Draw Options That Matter

`Renderer3D.DrawTriangles3D` creates `DrawTrianglesShaderOptions` with:

```go
RawDstCoordinates: true
Images:            opts.Images
Uniforms:          opts.Uniforms
```

`RawDstCoordinates` is important. It disables Ebitengine's sub-pixel adjustment for destination coordinates, which is correct when the vertex shader writes clip-space positions for 3D. If this is off, 3D geometry can be subtly shifted or distorted.

The custom `ProjectionMatrix` option on `DrawTrianglesShaderOptions` is separate from the built-in 3D `MVP` uniform. The prototype shader currently carries its own projection through `MVP`, so callers generally leave `ProjectionMatrix` nil.

## Backend Behavior

Metal and OpenGL currently implement the 3D mode hooks.

Metal:

* Implements `DepthTextureAttacher` and `DrawTrianglesWithMode`.
* Allocates a dedicated depth texture for the destination image.
* Uses a 3D render pass with color clear and depth clear.
* Enables depth compare `LessEqual` and depth writes.
* Uses logical image size for the 3D viewport instead of padded internal texture size.
* Enables back-face culling with clockwise front-facing winding.
* Rejects non-`FillRuleFillAll` 3D draws.

OpenGL / WebGL:

* Implements `DepthTextureAttacher` and `DrawTrianglesWithMode`.
* Allocates a depth renderbuffer and attaches it to the destination framebuffer.
* Enables `GL_DEPTH_TEST` for 3D draws and disables it afterward.
* Clears color and depth once per frame for each 3D target.
* Uses logical image size for the 3D viewport.
* WebGL requests a context with `depth: true` and `stencil: true`.
* Canonicalizes 3D draws via the `ebiten_3d_adjust` vertex-shader epilogue:
  y is flipped so NDC +y is the image top (matching Metal/DirectX), and the
  canonical clip z in `[0, w]` is remapped to OpenGL's `[-w, +w]`. The
  uniform is driver-owned and 0 for 2D draws (identity).
* Back-face culling is not currently enabled in the OpenGL 3D path.

DirectX:

* Implements `DepthTextureAttacher` and `DrawTrianglesWithMode` for both Direct3D 11 and Direct3D 12.
* Allocates a D24S8 depth-stencil resource for depth-enabled render targets.
* Enables depth compare `LessEqual` and depth writes only for `DrawMode3D`.
* Clears color and depth once per frame for each 3D target.
* Uses logical image size for the 3D viewport.
* Enables back-face culling with clockwise front-facing winding.
* Rejects non-`FillRuleFillAll` 3D draws.

## Extending 3D Rendering

When adding a new 3D feature, keep the separation between 2D and 3D explicit:

* Add public options to `Renderer3D` or `DrawTriangles3DOptions` only when they map clearly to backend state or shader data.
* Keep backend state changes scoped to `DrawMode3D`.
* Do not enable depth testing globally.
* Do not change stencil behavior used by vector rendering unless the change is also correct for normal 2D fills.
* Prefer offscreen 3D targets and 2D composition over drawing depth-tested content directly to the screen.
* Keep the default 2D batching path on `DrawModeDefault`.

If you add backend state such as culling, depth compare functions, wireframe, or depth write toggles, update:

* `internal/graphicsdriver/graphics.go` if the cross-backend contract changes.
* `internal/graphicscommand` if commands need to carry new mode data.
* Every backend that implements `DrawTrianglesWithMode`.
* `Renderer3D` or `DrawTriangles3DOptions` if the option should be public.
* `examples/mesh` or another small sample so behavior can be visually checked.

## Troubleshooting

If 2D sprites, UI, or vector graphics disappear after 3D rendering:

* Check that the 3D draw target is offscreen and depth-enabled, not the final screen image.
* Check that the backend disables depth testing after `DrawMode3D`.
* Check that 2D draws still go through `DrawModeDefault`.
* Check that stencil state is restored or isolated; vector fills rely on stencil.

If 3D geometry appears shifted, stretched, or off-center:

* Verify `RawDstCoordinates: true` for the 3D shader draw.
* Verify the backend uses logical target size for 3D viewports, not padded internal texture size.
* Verify the aspect ratio comes from `Renderer3D.Aspect()` after `Begin` has resized the target.

If depth ordering is wrong:

* Confirm `EnableDepthBuffer` is called on the render target before drawing.
* Confirm the backend implements `DrawTrianglesWithMode`; otherwise the draw falls back to the normal 2D path without depth state.
* Confirm the depth buffer is cleared at the start of the 3D pass or once per frame for the target.
* Confirm clip-space `z` values are in the backend's expected range.

If lighting rotates with the mesh:

* Confirm normals are transformed by `NormalM` in the shader.
* For pure rotations, pass the model rotation as `NormalM`.
* For non-uniform scale, pass inverse-transpose normal data.
* Confirm `LightDir` is in the same space as transformed normals.

If the wrong side of a mesh is visible or culled:

* Check triangle winding. The mesh example emits clockwise front faces for the current Metal culling setup.
* Remember that OpenGL currently does not enable culling in 3D mode, so culling bugs might only appear on Metal.
* Keep winding and cull mode consistent when adding OpenGL or DirectX culling.

## Verification

Useful checks while changing 3D rendering:

```sh
go build ./examples/mesh
go test ./internal/graphicsdriver/opengl/...
GOOS=js GOARCH=wasm go test ./internal/graphicsdriver/opengl
GOOS=darwin go test ./internal/graphicsdriver/metal/...
```

Manual verification is still important. Run `go run ./examples/mesh` and check that:

* The cube depth-sorts correctly as it rotates.
* Lighting changes with face orientation when `LightDir` is fixed.
* 2D debug text draws after the 3D pass.
* Vector or other stencil-using 2D content still renders after the 3D composite.
