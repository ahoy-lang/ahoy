# Array Methods Implementation Plan

## Approach: Generate C Helper Functions Inline

Instead of a separate runtime library, we'll generate the helper functions
at the top of each C file when array methods are used.

## Array Methods to Implement:

1. **length||** - Return array size
2. **push|value|** - Append to array
3. **pop||** - Remove and return last element
4. **sum||** - Sum all elements
5. **sort||** - Sort array
6. **reverse||** - Reverse array
7. **shuffle||** - Randomize array
8. **pick||** - Get random element
9. **has|value|** - Check if value exists
10. **map|lambda|** - Transform each element (needs lambda support)
11. **filter|lambda|** - Filter elements (needs lambda support)

## Implementation Strategy:

1. Track which array methods are used in the AST
2. Generate only the needed helper functions
3. Add them to the C file header (after includes, before main)
4. Use a simple dynamic array structure

## C Structure:

```c
typedef struct {
    int* data;
    int length;
    int capacity;
} AhoyArray;

AhoyArray* ahoy_array_create() { ... }
int ahoy_array_length(AhoyArray* arr) { ... }
void ahoy_array_push(AhoyArray* arr, int value) { ... }
int ahoy_array_pop(AhoyArray* arr) { ... }
// etc.
```
