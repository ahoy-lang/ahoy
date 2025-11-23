

#ifndef MYHEADER_H
#define MYHEADER_H

// Function declaration
int add(int a, int b);


// Vector2, 2 components
typedef struct Vector2 {
    float x;
    float y;
} Vector2;

typedef struct Color {
    unsigned char r;
    unsigned char g;
    unsigned char b;
    unsigned char a;
} Color;

#endif // MYHEADER_H
