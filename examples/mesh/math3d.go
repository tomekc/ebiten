package main

import "math"

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
