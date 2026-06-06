What’s biting you here is that you’ve taken over the *global* depth-stencil state in an engine that was built assuming “no depth at all, just painter’s algorithm + stencil for vector graphics”. So once you flip depth on for your 3D triangles, you’ve accidentally changed the rules for all the 2D stuff that comes after.

Ebitengine *does* rely on stencil buffers internally for vector graphics and complex fills. Since v2.2 the vector API is implemented with stencil buffers, and v2.9 refined this with a custom stencil pass.([ebitengine.org][1]) If you’re hacking the depth/stencil attachment or state globally, you’re stepping directly on that codepath.

Let me break it down and propose a way to structure your “3D mode” so it doesn’t pollute normal 2D rendering.

---

## What’s probably going wrong

Likely one or more of these:

1. **You’re enabling depth test on the main render pass and not turning it off.**
   Ebitengine’s regular quads/triangles now get depth-tested against whatever your 3D draw left in the depth buffer → sprites disappear or appear in the wrong order.

2. **You’re changing the depth-stencil attachment that Ebitengine expects for 2D / vector graphics.**
   Since vector fills use stencil operations, altering depth/stencil format or ops (e.g. non-default compare/op) will break those fills or cause “holes” in shapes.([ebitengine.org][1])

3. **You’re using the same depth-stencil texture for “3D depth” and “2D stencil”.**
   Depth+stencil live in a *single* texture in the underlying APIs. If you start writing depth and changing depth-stencil state without coordinating with the stencil side, you get hard-to-debug interactions.

---

## High-level design that *won’t* mess up 2D

Instead of globally flipping a “3D mode flag” on the main screen, treat 3D as **its own render pass with its own depth buffer**, then composite that back into the regular 2D pipeline.

Conceptually:

1. **3D pass** (offscreen, with depth+back-face culling)
2. **2D pass** (the normal Ebitengine path, *no depth*)

```text
[Your 3D renderer] ──> color RT A + depth RT A
                       ↓
               Draw RT A as a normal Image onto screen
               + all your usual 2D sprites / UI / vector stuff
```

That gives you:

* A real depth buffer and culling for 3D
* Zero changes to how 2D uses stencil / blending / ordering

You can see a similar philosophy in Tetra3D, which piggybacks on Ebitengine but manages its own camera, matrices and depth buffer while still rendering via Ebitengine images and shaders.([GitHub][2])

---

## Concretely: how to structure the “3D mode”

### 1. Separate render targets

Inside your fork / internal graphics layer, introduce a “3D render target”:

* Color texture (same size as screen or whatever)
* Depth-stencil texture (format suitable for depth, e.g. Depth24/32+Stencil)

Use this **only** when 3D mode is enabled, not for the normal 2D draw pipeline.

**3D mode flow:**

1. End / flush any batched 2D draw calls.
2. Begin a render pass on the 3D RT:

    * `colorAttachment = color3D`
    * `depthStencilAttachment = depth3D`
    * `loadOp` for depth: clear to 1.0
    * `storeOp`: store
3. Use pipelines with:

    * Depth test: enabled, compare: `LessEqual`
    * Depth write: enabled
    * Stencil: disabled or “keep” on all ops
    * Cull mode: back
4. Draw your 3D triangles.
5. End that pass.

Then, in your normal Ebitengine `Draw`:

6. Draw `color3D` onto the main screen as a regular `*ebiten.Image`.
7. Continue drawing all 2D sprites, tilemaps, vector graphics as usual (no depth).

From the public API you can expose this as something like:

```go
// pseudo
type Renderer3D struct {
    Target *ebiten.Image // the 3D color RT
    // internal depth, pipelines, etc.
}

func (r *Renderer3D) Begin() { /* sets up 3D pass */ }
func (r *Renderer3D) End()   { /* finishes 3D pass   */ }
func (r *Renderer3D) DrawTriangles(verts []Vertex3D, idx []uint16) { ... }
```

And in your game’s `Draw`:

```go
func (g *Game) Draw(screen *ebiten.Image) {
    // 1) render 3D world into offscreen
    g.renderer3D.Begin()
    g.renderer3D.DrawWorld()
    g.renderer3D.End()

    // 2) composite into screen
    screen.DrawImage(g.renderer3D.Target, nil)

    // 3) draw HUD / 2D etc.
    // all the usual 2D Ebiten calls
}
```

No 2D code ever sees depth.

---

### 2. Depth-stencil *state* separation

If you *really* want to use depth on the main screen and not add an offscreen pass, you **must** guarantee:

* All **2D pipelines** have:

    * `depthTest = disabled`
    * `depthWrite = disabled`
    * `stencilTest = disabled` or exactly matching what the vector-graphics code expects
* All **3D pipelines** have depth enabled, but:

    * stencil ops set to `KEEP` for all cases
    * stencil test disabled

So your mode switch becomes:

```go
func enter3DMode() {
    // switch to pipelines with depth on, stencil untouched
}

func exit3DMode() {
    // switch back to pipelines with depth off
    // optionally clear depth to 1.0 so any accidental later depth use won’t matter
}
```

The catch: Ebitengine aggressively batches draw calls and caches pipelines, so mixing “2D pipeline state” and “3D pipeline state” in the same pass might fight with that batching model. The offscreen-pass approach is usually cleaner and easier to reason about.

---

### 3. Don’t fight the stencil

Because vector graphics are implemented with stencil buffers and even-odd fills, touching stencil is dangerous:([ebitengine.org][1])

* **Don’t** rely on the stencil aspect of the depth-stencil texture for your own purposes.
* **Do** configure your 3D pipelines to:

    * never write stencil (`writeMask = 0`)
    * use stencil compare `ALWAYS` and ops `KEEP` if stencil is enabled at all.

That way, you only use the depth aspect; you never clobber what 2D vector rendering wrote into stencil.

---

## If you’d rather stay inside user-space (no engine fork)

If you decide not to modify Ebitengine’s internals at all, your options are basically:

1. **Software / shader depth buffer**
   Use an extra `*ebiten.Image` as a “depth texture” and manually do depth tests in a shader (what Tetra3D does).([GitHub][2])

    * Pros: no changes to engine; portable across all backends.
    * Cons: more math in fragment shader, less efficient than hardware depth.

2. **Use an existing 3D-on-Ebitengine lib**
   Tetra3D already solved most of this (camera, matrices, depth handling, etc.) and still integrates naturally with 2D Ebitengine rendering. Looking at its source architecture might give you concrete patterns to borrow for your own experiment.

---

## TL;DR: practical checklist

* ✅ Don’t flip depth test “on globally” for the screen pass.
* ✅ Do render 3D into its **own color+depth target**, then blit that as a 2D image.
* ✅ Do use pipelines with depth test ON only for 3D, OFF for 2D.
* ✅ Do leave stencil alone or in KEEP/disabled mode for 3D.
* ✅ Do clear your 3D depth buffer when starting a 3D pass.

If you want, next step I can help sketch actual code / internal API changes for:

* the offscreen 3D target + depth buffer
* simple camera + projection matrices
* a “3D triangles” call that plays nice with Ebitengine’s batching model.

[1]: https://ebitengine.org/en/documents/2.2.html?utm_source=chatgpt.com "Ebiten 2.2 Release Notes"
[2]: https://github.com/SolarLune/tetra3d?utm_source=chatgpt.com "SolarLune/tetra3d"
