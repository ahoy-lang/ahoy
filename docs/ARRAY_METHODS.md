
# functional methods for arrays

# length of array
```ahoy
sum  : [4,5,5,2].length||
print(new_array)  ? Outputs: 4
```
? pop removes and returns the last element of the array
? push adds an element to the end of the array
```ahoy
last_element_array  : [4,5,5,2].pop||
print(last_element_array)  ? Outputs: 2
long_array  : [4,5,5].push|2|
print(long_array)  ? Outputs: [4, 5, 5, 2]
```

# shuffle randomizes the order of elements in the array
```ahoy
shuffled_array  : [4,5,5,2].shuffle||
print(shuffled_array)  ? Outputs: [5, 2, 4, 5] (example output, actual order may vary)
```

# pick random element from the array
```ahoy
random_element  : [4,5,5,2].pick||
print(random_element)  ? Outputs: 5 (example output, actual element may vary)
```

# method chaining
```ahoy
result  : [4,5,5,2].push|3|.shuffle||.pick||
print(result)  ? Outputs: 3 (example output, actual element may vary)

# sum accumulates the sum of all elements in the array
```ahoy
sum  : [4,5,5,2].sum||
print(new_array)  ? Outputs: 16
```
# map works like javascripts map function returning a new array
```ahoy
new_array  : [4,5,5,2].map|element: element * 2|
print(new_array)  ? Outputs: [8, 10, 10, 4]
```

# filter works like javascripts filter function
```ahoy
new_array  : array.map|element: element > 2|
print(new_array)  ? Outputs: [4, 5, 5]
```

# sort sorts the array in ascending order
```ahoy
new_array  : [4,5,5,2].sort||
print(new_array)  ? Outputs: [2, 4, 5, 5]
```

# reverse reverses the order of elements in the array
```ahoy
new_array  : [4,5,5,2].reverse||
print(new_array)  ? Outputs: [2, 5, 5, 4]
```

# has: checks if the array contains a specific element
```ahoy
has_five  : [4,5,5,2].has|5||
print(contains_five)  ? Outputs: true
has_bob  : ["ciril", "alice", "bob"].has|"bob"||
print(has_bob)  ? Outputs: true
```
