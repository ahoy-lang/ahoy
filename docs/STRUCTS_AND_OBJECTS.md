# example of struct declarations
```ahoy
struct particle:
  position: vector2
  velocity: vector2
  rotation: float
  type smoke_particle:
	  size: float
	  alpha: float
	  life: float
	  max_life: float
	  color: color
  type wind_particle:
    direction: vector2
    speed: float
    size: vector2
```


# optional struct name following struct keyword; example:
struct particle

# example of struct initialization to make an object of type smoke_particle
smoke_particle1: smoke_particle<position:vector2|120,390|, size: 10.0, alpha: 1.0, life:1.0>


# accessing struct properties
smoke_particle1.max_life: 1.0
smoke_particle1<'max_life'>: 100.0
smoke_particle1.position.x: 1.0
smoke_particle1.position.y: 1.0
smoke_particle1<'position.x'>: 100.0

**Features:**
- initialize objects built from structs with default values unless value provided e.g int to 0, float to 0.0, string to "", bool to false, Vector2|0,0| , color to color| 200, 200, 200, 255 | , and custom structs to their zeroed state.
- struct type inherits from the struct properties above it; so in this example smoke_particle inherits position and velocity from struct and cookie_truck also inherits position and velocity from struct
- access struct properties with dot notation or angle brackets with property name as string
- C doesnt have vector2 type so we should typedef struct for vector2 in the generated C code

# examples of struct with syntax on one line
struct particles: position: vector2; velocity: vector2
  type smoke_particle: size: float; alpha: float; life: float; max_life: float; color: color
  type cookie_truck: direction: vector2; speed: float; size: vector2
