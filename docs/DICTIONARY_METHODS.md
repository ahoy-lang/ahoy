

# Dictionary Methods Reference

This document describes all available methods for dictionary objects in Ahoy.

## Basic Methods

### `.size()`
Returns the number of key-value pairs in the dictionary.

**Syntax:**
```ahoy
count : my_dict.size()
```

**Returns:** `int` - The number of entries in the dictionary

**Example:**
```ahoy
data : {"name": "Alice", "age": 30, "city": "NYC"}
print | data.size() |  ? Output: 3
```

---

### `.clear()`
Removes all entries from the dictionary, setting size to 0.

**Syntax:**
```ahoy
my_dict.clear()
```

**Returns:** `void`

**Example:**
```ahoy
data : {"a": 1, "b": 2, "c": 3}
print | data.size() |  ? Output: 3
data.clear()
print | data.size() |  ? Output: 0
```

---

### `.has(key)`
Checks if a key exists in the dictionary.

**Syntax:**
```ahoy
exists : my_dict.has(key)
```

**Parameters:**
- `key` - The key to search for

**Returns:** `bool` - `true` if key exists, `false` otherwise

**Example:**
```ahoy
person : {"name": "Bob", "age": 25}
print | person.has("name") |  ? Output: true
print | person.has("email") | ? Output: false
```

---

### `.has_all(keys)`
Checks if the dictionary contains all the specified keys.

**Syntax:**
```ahoy
has_all : my_dict.has_all(keys_array)
```

**Parameters:**
- `keys_array` - Array of keys to check

**Returns:** `bool` - `true` if all keys exist, `false` otherwise

**Example:**
```ahoy
person : {"name": "Charlie", "age": 30, "city": "LA"}
required : ["name", "age"]
print | person.has_all(required) |  ? Output: true

required2 : ["name", "email"]
print | person.has_all(required2) | ? Output: false
```

---

### `.keys()`
Returns an array containing all keys in the dictionary.

**Syntax:**
```ahoy
keys_array : my_dict.keys()
```

**Returns:** `array` - Array of all dictionary keys

**Example:**
```ahoy
data : {"x": 10, "y": 20, "z": 30}
keys : data.keys()
loop key in keys do
    print | key |  ? Output: x, y, z (order not guaranteed)
```

---

### `.values()`
Returns an array containing all values in the dictionary.

**Syntax:**
```ahoy
values_array : my_dict.values()
```

**Returns:** `array` - Array of all dictionary values

**Example:**
```ahoy
scores : {"math": 95, "english": 88, "science": 92}
vals : scores.values()
loop val in vals do
    print | val |  ? Output: 95, 88, 92 (order not guaranteed)
```

---

## Sorting Methods

### `.sort()`
Sorts the dictionary by keys in ascending order.

**Syntax:**
```ahoy
sorted_dict : my_dict.sort()
```

**Returns:** `dict` - A new dictionary with keys sorted alphabetically

**Example:**
```ahoy
data : {"zebra": 1, "apple": 2, "mango": 3}
sorted : data.sort()
keys : sorted.keys()
loop key in keys do
    print | key |  ? Output: apple, mango, zebra
```

---

### `.stable_sort()`
Sorts the dictionary by keys using a stable sorting algorithm. This preserves the relative order of entries with equal keys (though dict keys are unique, this ensures consistent ordering).

**Syntax:**
```ahoy
sorted_dict : my_dict.stable_sort()
```

**Returns:** `dict` - A new dictionary with keys sorted alphabetically (stable)

**Example:**
```ahoy
data : {"c": 3, "a": 1, "b": 2}
sorted : data.stable_sort()
? Guaranteed stable ordering
```

---

## Merging Methods

### `.merge(other_dict)`
Merges another dictionary into this one. If keys exist in both, values from `other_dict` override.

**Syntax:**
```ahoy
merged : dict1.merge(dict2)
```

**Parameters:**
- `other_dict` - Dictionary to merge in

**Returns:** `dict` - A new dictionary with merged entries

**Example:**
```ahoy
defaults : {"host": "localhost", "port": 8080, "debug": false}
config : {"port": 3000, "timeout": 30}

final : defaults.merge(config)
? final = {"host": "localhost", "port": 3000, "debug": false, "timeout": 30}
```

---

## Complete Example

```ahoy
? Create a dictionary
users : {
    "alice": {"age": 30, "role": "admin"},
    "bob": {"age": 25, "role": "user"},
    "charlie": {"age": 35, "role": "user"}
}

? Check size
print | "Total users:" |
print | users.size() |

? Check if key exists
if users.has("alice") then
    print | "Alice exists!" |

? Get all usernames
usernames : users.keys()
print | "Usernames:" |
loop name in usernames do
    print | name |

? Merge with additional users
new_users : {"diana": {"age": 28, "role": "admin"}}
all_users : users.merge(new_users)
print | "Total after merge:" |
print | all_users.size() |

? Clear dictionary
users.clear()
print | "After clear:" |
print | users.size() |  ? Output: 0
```

---

## Method Summary Table

| Method | Returns | Description |
|--------|---------|-------------|
| `.size()` | `int` | Get number of entries |
| `.clear()` | `void` | Remove all entries |
| `.has(key)` | `bool` | Check if key exists |
| `.has_all(keys)` | `bool` | Check if all keys exist |
| `.keys()` | `array` | Get all keys |
| `.values()` | `array` | Get all values |
| `.sort()` | `dict` | Sort by keys (ascending) |
| `.stable_sort()` | `dict` | Stable sort by keys |
| `.merge(dict)` | `dict` | Merge two dictionaries |

---

## Notes

1. **Immutability**: Methods like `.sort()` and `.merge()` return NEW dictionaries
2. **Mutability**: `.clear()` modifies the dictionary in place
3. **Key Order**: Dictionary key order is not guaranteed except after sorting
4. **Type Safety**: All methods are type-checked at compile time
5. **Performance**: `.has()` is O(1), `.keys()` and `.values()` are O(n)

---

## LSP Autocomplete Support

When typing a dot (`.`) after a dictionary variable, the Language Server Protocol provides autocomplete suggestions for all available methods:

```ahoy
my_dict : {"a": 1}
my_dict.  ? <-- Autocomplete shows: size, clear, has, has_all, keys, values, sort, stable_sort, merge
```


# example: LINQ Query Syntax in C#

# Dictionary Query Methods
Filtering: Where
Ordering: order_by, order_desc, order_asc()
Projection: select (to transform objects or select specific properties)
Grouping: group_by
Aggregation: count, sum, min, max, avg
Quantifiers: any, all
Set Operators: distinct, union, intersect, except

# query method should support dictionary arrays and object arrays only
# this is because we can just use filter method for primitive arrays

```ahoy
array_of_people: [
		{"first_name":"Alice", "last_name":"Smith", "Age":30},
		{"first_name":"Bob", "last_name":"Johnson", "Age":25},
		{"first_name":"Charlie", "last_name":"Brown", "Age":40},
		{"first_name":"Diana", "last_name":"Smith", "Age":20}
]
query_result: array_of_people.query|person: where "age" > 25; order_by "last_name"; select person|

print|"People older than 25 ordered by last name:\n"|
loop person in query_result do
		print|"%s %s, Age: %d\n", person{"first_name"}, person{"last_name"}, person{"Age"}|
		? Output: Alice Smith, Age: 30 Charlie Brown, Age: 40
