package main

// surfaceShader details every generated surface. The mesh's UV channel
// carries the meshgen.Material in x and a wind-sway weight in y; colours are
// sRGB vertex colours. Detail is procedural and in world space, so no
// textures or tangents are needed: height fields are turned into normals
// with screen-space derivatives.
const surfaceShader = `
shader_type spatial;
render_mode blend_mix, depth_draw_opaque, cull_back, diffuse_burley, specular_schlick_ggx;

uniform float wind = 1.0;

varying vec3 wpos;
varying flat int mat;

float hash(vec3 p) {
	p = fract(p * 0.3183099 + vec3(0.71, 0.113, 0.419));
	p *= 17.0;
	return fract(p.x * p.y * p.z * (p.x + p.y + p.z));
}

float noise(vec3 x) {
	vec3 i = floor(x);
	vec3 f = fract(x);
	f = f * f * (3.0 - 2.0 * f);
	return mix(mix(mix(hash(i), hash(i + vec3(1, 0, 0)), f.x),
	               mix(hash(i + vec3(0, 1, 0)), hash(i + vec3(1, 1, 0)), f.x), f.y),
	           mix(mix(hash(i + vec3(0, 0, 1)), hash(i + vec3(1, 0, 1)), f.x),
	               mix(hash(i + vec3(0, 1, 1)), hash(i + vec3(1, 1, 1)), f.x), f.y), f.z);
}

float fbm(vec3 p) {
	float v = 0.0;
	float a = 0.5;
	for (int i = 0; i < 4; i++) {
		v += a * noise(p);
		p = p * 2.03 + vec3(1.7, 9.2, 3.1);
		a *= 0.5;
	}
	return v;
}

vec3 to_linear(vec3 c) {
	return mix(c / 12.92, pow((c + 0.055) / 1.055, vec3(2.4)), step(0.04045, c));
}

void vertex() {
	mat = int(UV.x + 0.5);
	if (mat == 20) {
		// Grass tufts shrink into the lawn before their visibility cutoff.
		float cd = length((MODELVIEW_MATRIX * vec4(0.0, 0.0, 0.0, 1.0)).xyz);
		VERTEX *= smoothstep(64.0, 44.0, cd);
	}
	vec3 w = (MODEL_MATRIX * vec4(VERTEX, 1.0)).xyz;
	float sway = UV.y;
	if (sway > 0.0) {
		float t = TIME * wind;
		vec3 off = vec3(sin(t * 1.1 + w.x * 0.11 + w.z * 0.05), 0.0, cos(t * 0.9 + w.z * 0.09)) * 0.14;
		off += vec3(sin(t * 4.3 + w.y * 1.9 + w.x), 0.4 * sin(t * 3.1 + w.z * 2.3), cos(t * 3.7 + w.x * 1.3)) * 0.035;
		off *= sway;
		VERTEX += (inverse(MODEL_MATRIX) * vec4(off, 0.0)).xyz;
		w += off;
	}
	wpos = w;
}

// bump perturbs the view-space normal n by the height field h (metres).
vec3 bump(vec3 n, vec3 view_pos, float h) {
	vec3 dpdx = dFdx(view_pos);
	vec3 dpdy = dFdy(view_pos);
	float hx = dFdx(h);
	float hy = dFdy(h);
	vec3 r1 = cross(dpdy, n);
	vec3 r2 = cross(n, dpdx);
	float det = dot(dpdx, r1);
	vec3 g = sign(det) * (hx * r1 + hy * r2);
	return normalize(abs(det) * n - g);
}

void fragment() {
	vec3 c = to_linear(COLOR.rgb);
	vec3 wn = normalize((INV_VIEW_MATRIX * vec4(NORMAL, 0.0)).xyz);
	float dist = length(VERTEX);
	float near = clamp(1.0 - dist / 70.0, 0.0, 1.0); // fine detail fades out
	float h = 0.0;
	float rough = 0.8;
	float metal = 0.0;
	float spec = 0.5;
	vec3 emit = vec3(0.0);
	vec3 back = vec3(0.0);
	float coat = 0.0;

	// A horizontal coordinate along vertical walls, and height up them.
	vec2 side = normalize(vec2(-wn.z, wn.x) + vec2(1e-5));
	float u = dot(wpos.xz, side);
	float v = wpos.y;
	// Fade periodic patterns as they approach the pixel size, to avoid moiré.
	float px = max(fwidth(u), fwidth(v));
	float aa = clamp(1.0 - (px - 0.01) / 0.03, 0.0, 1.0);

	if (mat == 1) { // grass
		float big = fbm(wpos * 0.21);
		float fine = noise(wpos * 5.7);
		float blades = noise(wpos * vec3(31.0, 1.0, 29.0));
		c *= mix(0.7, 1.12, big) * (0.86 + 0.26 * fine) * mix(1.0, 0.8 + 0.35 * blades, near);
		c *= mix(0.82, 1.0, smoothstep(0.2, 0.6, fbm(wpos * 0.9 + vec3(5.0))));
		float dry = smoothstep(0.58, 0.78, fbm(wpos * 0.045 + vec3(3.0)));
		c = mix(c, c * vec3(1.35, 1.15, 0.55), dry * 0.45);
		h = near * (0.012 * fine + 0.006 * noise(wpos * 23.0));
		rough = 0.95;
		spec = 0.3;
	} else if (mat == 2) { // asphalt
		float grain = noise(wpos * 8.0);
		float stone = noise(wpos * 37.0);
		c *= (0.84 + 0.22 * grain) * mix(0.88, 1.1, fbm(wpos * 0.07));
		c += vec3(0.05) * step(0.9, stone) * near;
		float patchy = smoothstep(0.66, 0.7, fbm(wpos * 0.11 + vec3(7.0)));
		c *= 1.0 - 0.18 * patchy;
		h = near * (0.004 * stone + 0.003 * grain);
		rough = mix(0.86, 0.7, patchy);
	} else if (mat == 3) { // concrete
		c *= (0.9 + 0.12 * fbm(wpos * 0.8)) * mix(1.0, 0.84, smoothstep(0.55, 0.8, fbm(wpos * 0.19)));
		h = near * 0.002 * noise(wpos * 21.0);
		rough = 0.9;
	} else if (mat == 4) { // brick
		if (abs(wn.y) < 0.5) {
			float row = floor(v / 0.086);
			float uu = u + mod(row, 2.0) * 0.115;
			float col = floor(uu / 0.23);
			vec2 f = vec2(fract(uu / 0.23) * 0.23, fract(v / 0.086) * 0.086);
			float edge = min(min(f.x, 0.23 - f.x), min(f.y, 0.086 - f.y));
			float mortar = 1.0 - smoothstep(0.006, 0.011, edge);
			float tone = hash(vec3(row, col, 3.0));
			c *= mix(1.0, 0.72 + 0.5 * tone, aa * 0.8 + 0.2);
			c = mix(c, vec3(0.62, 0.58, 0.52), mortar * aa * 0.9);
			h = -0.006 * mortar * near * aa;
		}
		c *= 0.92 + 0.12 * noise(wpos * 11.0);
		rough = 0.92;
	} else if (mat == 5) { // weatherboard
		if (abs(wn.y) < 0.5) {
			float f = fract(v / 0.17);
			c *= mix(0.92, 0.84 + 0.16 * f - 0.18 * (1.0 - smoothstep(0.0, 0.07, f)), aa);
			h = 0.012 * f * near * aa;
		}
		c *= 0.96 + 0.06 * noise(wpos * 4.0);
		rough = 0.7;
	} else if (mat == 6) { // rendered masonry
		c *= 0.93 + 0.1 * fbm(wpos * 2.5);
		h = near * 0.0025 * noise(wpos * 29.0);
		rough = 0.9;
	} else if (mat == 7) { // terracotta tiles
		float col = floor(u / 0.3);
		float row = floor(v / 0.13 + hash(vec3(col, 1.0, 2.0)) * 0.2);
		float f = fract(v / 0.13);
		float arch = sin(fract(u / 0.3) * 3.14159);
		c *= mix(1.0, 0.78 + 0.35 * hash(vec3(col, row, 5.0)), aa * 0.7 + 0.3);
		c *= mix(0.9, (0.8 + 0.2 * arch) * (1.0 - 0.25 * (1.0 - smoothstep(0.0, 0.18, f))), aa);
		c *= 0.9 + 0.15 * fbm(wpos * 0.9); // weathering
		h = near * aa * (0.018 * arch + 0.012 * f);
		rough = 0.75;
	} else if (mat == 8) { // corrugated steel
		float rib = pow(abs(sin(u * 3.14159 / 0.19)), 6.0);
		h = near * aa * 0.012 * rib;
		c *= mix(0.95, 0.9 + 0.1 * rib, aa);
		c *= 0.92 + 0.12 * fbm(wpos * 0.6); // weathering
		metal = 0.12;
		rough = 0.58;
		spec = 0.4;
	} else if (mat == 9) { // glass
		c *= 0.55;
		// A hint of curtains behind some windows.
		float curtain = step(0.55, hash(floor(wpos * 0.5)));
		c = mix(c, vec3(0.45, 0.4, 0.33) * 0.35, curtain * smoothstep(0.3, 0.9, fract(u * 0.8)) * 0.6);
		rough = 0.04;
		metal = 0.15;
		spec = 1.0;
	} else if (mat == 10) { // foliage
		c *= (0.72 + 0.42 * noise(wpos * 1.6)) * (0.86 + 0.28 * noise(wpos * 6.5));
		back = c * 0.55;
		h = near * 0.05 * noise(wpos * 4.5);
		rough = 0.85;
		spec = 0.3;
	} else if (mat == 20) { // grass blades
		c *= mix(0.72, 1.12, fbm(wpos * 0.21)) * (0.85 + 0.3 * noise(wpos * 3.0));
		back = c * 0.6;
		rough = 0.9;
		spec = 0.25;
	} else if (mat == 11) { // bark
		float n = noise(vec3(wpos.x * 9.0, wpos.y * 0.9, wpos.z * 9.0));
		c *= 0.7 + 0.45 * n;
		h = near * 0.01 * n;
		rough = 0.95;
	} else if (mat == 12) { // vehicle paint
		c *= mix(0.72, 1.0, smoothstep(0.2, 1.3, wpos.y)); // road grime low down
		rough = 0.3;
		metal = 0.1;
		coat = 1.0;
		spec = 0.6;
	} else if (mat == 13) { // plastic
		c *= 0.95 + 0.08 * noise(wpos * 13.0);
		h = near * 0.0008 * noise(wpos * 60.0);
		rough = 0.48;
		spec = 0.45;
	} else if (mat == 14) { // road paint
		float wear = smoothstep(0.5, 0.8, noise(wpos * 2.7) * 0.6 + noise(wpos * 17.0) * 0.4);
		c = mix(c, vec3(0.06), wear * 0.7);
		rough = 0.6;
	} else if (mat == 15) { // lamps
		emit = c * 2.5;
	} else if (mat == 16) { // rubber
		rough = 0.9;
		spec = 0.3;
	} else if (mat == 17) { // bare metal
		metal = 0.85;
		rough = 0.28;
	} else if (mat == 18) { // solar panel
		vec2 cell = fract(vec2(u / 0.16, v / 0.1));
		float grid = 1.0 - smoothstep(0.0, 0.08, min(min(cell.x, 1.0 - cell.x), min(cell.y, 1.0 - cell.y)));
		c = mix(vec3(0.02, 0.04, 0.12), vec3(0.6), grid * 0.6 * aa + 0.08 * (1.0 - aa));
		rough = 0.12;
		metal = 0.3;
		spec = 1.0;
	} else if (mat == 19) { // interior plastic
		h = near * 0.0006 * noise(wpos * 90.0);
		c *= 0.95 + 0.1 * noise(wpos * 7.0);
		rough = 0.7;
	}

	if (h != 0.0) {
		NORMAL = bump(NORMAL, VERTEX, h);
	}
	ALBEDO = c;
	ROUGHNESS = rough;
	METALLIC = metal;
	SPECULAR = spec;
	EMISSION = emit;
	BACKLIGHT = back;
	CLEARCOAT = coat;
	CLEARCOAT_ROUGHNESS = 0.1;
}
`

// skyShader paints a sunny sky with fair-weather clouds. It
// does not use TIME, so the engine only bakes its reflections once.
const skyShader = `
shader_type sky;

uniform vec3 zenith = vec3(0.16, 0.36, 0.78);
uniform vec3 horizon = vec3(0.66, 0.78, 0.9);
uniform vec3 below = vec3(0.34, 0.4, 0.3);

float h2(vec2 p) {
	p = fract(p * vec2(0.1031, 0.1030));
	p += dot(p, p.yx + 33.33);
	return fract((p.x + p.y) * p.x);
}

float n2(vec2 p) {
	vec2 i = floor(p);
	vec2 f = fract(p);
	f = f * f * (3.0 - 2.0 * f);
	return mix(mix(h2(i), h2(i + vec2(1, 0)), f.x), mix(h2(i + vec2(0, 1)), h2(i + vec2(1, 1)), f.x), f.y);
}

float clouds(vec2 p) {
	float v = 0.0;
	float a = 0.55;
	for (int i = 0; i < 6; i++) {
		v += a * n2(p);
		p = p * 2.07 + vec2(3.1, 1.7);
		a *= 0.5;
	}
	return v;
}

void sky() {
	vec3 d = normalize(EYEDIR);
	float y = d.y;
	vec3 col = mix(horizon, zenith, pow(clamp(y, 0.0, 1.0), 0.45));
	col = mix(col, below, smoothstep(0.0, -0.08, y));

	vec3 sun = LIGHT0_DIRECTION;
	float sd = max(dot(d, sun), 0.0);
	vec3 sun_col = LIGHT0_COLOR * LIGHT0_ENERGY;
	col += sun_col * (pow(sd, 900.0) * 18.0 + pow(sd, 60.0) * 0.25 + pow(sd, 6.0) * 0.08);

	if (y > 0.0) {
		vec2 uv = d.xz / (y + 0.12) * 1.4;
		float c = clouds(uv + vec2(4.0, 9.0));
		float cover = smoothstep(0.52, 0.78, c);
		float shade = clamp(clouds(uv * 1.1 + vec2(4.2, 9.1) - sun.xz * 0.05) - c + 0.55, 0.0, 1.0);
		vec3 cloud = mix(vec3(0.62, 0.66, 0.74), vec3(1.0, 0.98, 0.95), shade) * (0.85 + 0.25 * pow(sd, 4.0));
		col = mix(col, cloud, cover * smoothstep(0.0, 0.18, y) * 0.95);
	}
	COLOR = col;
}
`
