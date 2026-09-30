import { Vector3 } from 'three';
import type { Light } from '../formats/wmap';

/** Uniforms for the map's directional light, shared by its materials. */
export interface LightUniforms {
  lightDirection: { value: Vector3 };
  lightDiffuse: { value: Vector3 };
  lightAmbient: { value: Vector3 };
}

export function lightUniforms(l: Light): LightUniforms {
  return {
    lightDirection: { value: new Vector3(...l.direction).normalize() },
    lightDiffuse: { value: new Vector3(...l.diffuse) },
    lightAmbient: { value: new Vector3(...l.ambient) },
  };
}

/** A light with no map, for models shown alone. */
export function defaultLight(): LightUniforms {
  return {
    lightDirection: { value: new Vector3(-1, 2, 1).normalize() },
    lightDiffuse: { value: new Vector3(1, 1, 1) },
    lightAmbient: { value: new Vector3(0.3, 0.3, 0.3) },
  };
}

/** GLSL for RO's light: clamp(diffuse × max(0, n · l) + ambient). */
export const LIGHT_GLSL = /* glsl */ `
uniform vec3 lightDirection;
uniform vec3 lightDiffuse;
uniform vec3 lightAmbient;

vec3 roLight(vec3 normal) {
  float d = max(dot(normalize(normal), lightDirection), 0.0);
  return clamp(lightDiffuse * d + lightAmbient, 0.0, 1.0);
}
`;
