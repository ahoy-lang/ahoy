package main

// =============================================================================
// AHOY STANDARD LIBRARY - C Implementation Strings
// =============================================================================
// This file contains all built-in function implementations as C code strings.
// Each function is defined once here and used by codegen.go.
// The LSP also uses this file (via ahoy_stdlib.ahoy) for goto definition.
// =============================================================================

// StdlibFunc represents a stdlib function definition
type StdlibFunc struct {
	Name       string // Function name (e.g., "ahoy_array_length")
	Category   string // "array", "dict", "string", or "builtin"
	MethodName string // Short method name (e.g., "length")
	ReturnType string // C return type
	Params     string // Parameter signature for documentation
	Doc        string // Documentation string
	Code       string // C implementation code
}

// ArrayMethods contains all array method implementations
var ArrayMethods = map[string]StdlibFunc{
	"length": {
		Name:       "ahoy_array_length",
		Category:   "array",
		MethodName: "length",
		ReturnType: "int",
		Params:     "arr:array",
		Doc:        "Returns the number of elements in the array",
		Code: `int ahoy_array_length(AhoyArray* arr) {
    return arr->length;
}
`,
	},
	"push": {
		Name:       "ahoy_array_push",
		Category:   "array",
		MethodName: "push",
		ReturnType: "array",
		Params:     "arr:array, value:any",
		Doc:        "Adds an element to the end of the array",
		Code: `AhoyArray* ahoy_array_push(AhoyArray* arr, intptr_t value, AhoyValueType type) {
    if (arr->length >= arr->capacity) {
        arr->capacity = arr->capacity == 0 ? 4 : arr->capacity * 2;
        arr->data = realloc(arr->data, arr->capacity * sizeof(intptr_t));
        arr->types = realloc(arr->types, arr->capacity * sizeof(AhoyValueType));
    }
    arr->data[arr->length] = value;
    arr->types[arr->length] = type;
    arr->length++;
    return arr;
}
`,
	},
	"pop": {
		Name:       "ahoy_array_pop",
		Category:   "array",
		MethodName: "pop",
		ReturnType: "any",
		Params:     "arr:array",
		Doc:        "Removes and returns the last element",
		Code: `intptr_t ahoy_array_pop(AhoyArray* arr) {
    if (arr->length == 0) return 0;
    return arr->data[--arr->length];
}
`,
	},
	"sum": {
		Name:       "ahoy_array_sum",
		Category:   "array",
		MethodName: "sum",
		ReturnType: "int",
		Params:     "arr:array",
		Doc:        "Returns the sum of all numeric elements",
		Code: `int ahoy_array_sum(AhoyArray* arr) {
    int total = 0;
    for (int i = 0; i < arr->length; i++) {
        total += (int)arr->data[i];
    }
    return total;
}
`,
	},
	"has": {
		Name:       "ahoy_array_has",
		Category:   "array",
		MethodName: "has",
		ReturnType: "bool",
		Params:     "arr:array, value:any",
		Doc:        "Checks if the array contains a specific element",
		Code: `int ahoy_array_has(AhoyArray* arr, intptr_t value) {
    for (int i = 0; i < arr->length; i++) {
        if (arr->data[i] == value) return 1;
    }
    return 0;
}
`,
	},
	"sort": {
		Name:       "ahoy_array_sort",
		Category:   "array",
		MethodName: "sort",
		ReturnType: "array",
		Params:     "arr:array",
		Doc:        "Sorts the array in ascending order",
		Code: `int __ahoy_compare_ints(const void* a, const void* b) {
    return (*(intptr_t*)a - *(intptr_t*)b);
}

AhoyArray* ahoy_array_sort(AhoyArray* arr) {
    qsort(arr->data, arr->length, sizeof(intptr_t), __ahoy_compare_ints);
    return arr;
}
`,
	},
	"reverse": {
		Name:       "ahoy_array_reverse",
		Category:   "array",
		MethodName: "reverse",
		ReturnType: "array",
		Params:     "arr:array",
		Doc:        "Reverses the order of elements in the array",
		Code: `AhoyArray* ahoy_array_reverse(AhoyArray* arr) {
    for (int i = 0; i < arr->length / 2; i++) {
        intptr_t temp = arr->data[i];
        arr->data[i] = arr->data[arr->length - 1 - i];
        arr->data[arr->length - 1 - i] = temp;
    }
    return arr;
}
`,
	},
	"shuffle": {
		Name:       "ahoy_array_shuffle",
		Category:   "array",
		MethodName: "shuffle",
		ReturnType: "array",
		Params:     "arr:array",
		Doc:        "Randomly shuffles the array elements",
		Code: `AhoyArray* ahoy_array_shuffle(AhoyArray* arr) {
    srand(time(NULL));
    for (int i = arr->length - 1; i > 0; i--) {
        int j = rand() % (i + 1);
        intptr_t temp = arr->data[i];
        arr->data[i] = arr->data[j];
        arr->data[j] = temp;
    }
    return arr;
}
`,
	},
	"pick": {
		Name:       "ahoy_array_pick",
		Category:   "array",
		MethodName: "pick",
		ReturnType: "any",
		Params:     "arr:array",
		Doc:        "Returns a random element from the array",
		Code: `intptr_t ahoy_array_pick(AhoyArray* arr) {
    if (arr->length == 0) return 0;
    srand(time(NULL));
    return arr->data[rand() % arr->length];
}
`,
	},
	"fill": {
		Name:       "ahoy_array_fill",
		Category:   "array",
		MethodName: "fill",
		ReturnType: "array",
		Params:     "arr:array, value:any, count:int",
		Doc:        "Fills the array with the specified value repeated count times",
		Code: `AhoyArray* ahoy_array_fill(AhoyArray* arr, intptr_t value, AhoyValueType type, int count) {
    if (count <= 0) return arr;
    if (arr->capacity < count) {
        arr->capacity = count;
        arr->data = realloc(arr->data, arr->capacity * sizeof(intptr_t));
        arr->types = realloc(arr->types, arr->capacity * sizeof(AhoyValueType));
    }
    for (int i = 0; i < count; i++) {
        arr->data[i] = value;
        arr->types[i] = type;
    }
    arr->length = count;
    return arr;
}
`,
	},
}

// DictMethods contains all dictionary method implementations
var DictMethods = map[string]StdlibFunc{
	"size": {
		Name:       "ahoy_dict_size",
		Category:   "dict",
		MethodName: "size",
		ReturnType: "int",
		Params:     "d:dict",
		Doc:        "Returns the number of key-value pairs in the dictionary",
		Code: `int ahoy_dict_size(HashMap* dict) {
    return dict->count;
}
`,
	},
	"clear": {
		Name:       "ahoy_dict_clear",
		Category:   "dict",
		MethodName: "clear",
		ReturnType: "void",
		Params:     "d:dict",
		Doc:        "Removes all entries from the dictionary",
		Code: `void ahoy_dict_clear(HashMap* dict) {
    for (int i = 0; i < dict->capacity; i++) {
        if (dict->entries[i].key) {
            free(dict->entries[i].key);
            dict->entries[i].key = NULL;
            dict->entries[i].value = 0;
        }
    }
    dict->count = 0;
}
`,
	},
	"has": {
		Name:       "ahoy_dict_has",
		Category:   "dict",
		MethodName: "has",
		ReturnType: "bool",
		Params:     "d:dict, key:string",
		Doc:        "Checks if a key exists in the dictionary",
		Code: `int ahoy_dict_has(HashMap* dict, char* key) {
    return hashmap_get(dict, key) != 0;
}
`,
	},
	"has_all": {
		Name:       "ahoy_dict_has_all",
		Category:   "dict",
		MethodName: "has_all",
		ReturnType: "bool",
		Params:     "d:dict, keys:array[string]",
		Doc:        "Checks if all specified keys exist in the dictionary",
		Code: `int ahoy_dict_has_all(HashMap* dict, AhoyArray* keys) {
    for (int i = 0; i < keys->length; i++) {
        if (!hashmap_get(dict, (char*)keys->data[i])) return 0;
    }
    return 1;
}
`,
	},
	"keys": {
		Name:       "ahoy_dict_keys",
		Category:   "dict",
		MethodName: "keys",
		ReturnType: "array[string]",
		Params:     "d:dict",
		Doc:        "Returns an array containing all keys in the dictionary",
		Code: `AhoyArray* ahoy_dict_keys(HashMap* dict) {
    AhoyArray* arr = malloc(sizeof(AhoyArray));
    arr->length = 0;
    arr->capacity = dict->count > 0 ? dict->count : 1;
    arr->data = malloc(arr->capacity * sizeof(intptr_t));
    arr->types = malloc(arr->capacity * sizeof(AhoyValueType));
    arr->is_typed = 1;
    arr->element_type = AHOY_TYPE_STRING;
    for (int i = 0; i < dict->capacity; i++) {
        if (dict->entries[i].key) {
            arr->data[arr->length] = (intptr_t)dict->entries[i].key;
            arr->types[arr->length] = AHOY_TYPE_STRING;
            arr->length++;
        }
    }
    return arr;
}
`,
	},
	"values": {
		Name:       "ahoy_dict_values",
		Category:   "dict",
		MethodName: "values",
		ReturnType: "array",
		Params:     "d:dict",
		Doc:        "Returns an array containing all values in the dictionary",
		Code: `AhoyArray* ahoy_dict_values(HashMap* dict) {
    AhoyArray* arr = malloc(sizeof(AhoyArray));
    arr->length = 0;
    arr->capacity = dict->count > 0 ? dict->count : 1;
    arr->data = malloc(arr->capacity * sizeof(intptr_t));
    arr->types = malloc(arr->capacity * sizeof(AhoyValueType));
    arr->is_typed = 0;
    for (int i = 0; i < dict->capacity; i++) {
        if (dict->entries[i].key) {
            arr->data[arr->length] = dict->entries[i].value;
            arr->types[arr->length] = dict->entries[i].type;
            arr->length++;
        }
    }
    return arr;
}
`,
	},
	"sort": {
		Name:       "ahoy_dict_sort",
		Category:   "dict",
		MethodName: "sort",
		ReturnType: "dict",
		Params:     "d:dict",
		Doc:        "Sorts the dictionary by keys in ascending order",
		Code: `int __ahoy_compare_keys(const void* a, const void* b) {
    return strcmp(*(char**)a, *(char**)b);
}

HashMap* ahoy_dict_sort(HashMap* dict) {
    char** keys = malloc(dict->count * sizeof(char*));
    int idx = 0;
    for (int i = 0; i < dict->capacity; i++) {
        if (dict->entries[i].key) {
            keys[idx++] = dict->entries[i].key;
        }
    }
    qsort(keys, dict->count, sizeof(char*), __ahoy_compare_keys);
    HashMap* sorted = hashmap_create();
    for (int i = 0; i < dict->count; i++) {
        intptr_t val = hashmap_get(dict, keys[i]);
        AhoyValueType type = hashmap_get_type(dict, keys[i]);
        hashmap_set(sorted, keys[i], val, type);
    }
    free(keys);
    return sorted;
}
`,
	},
	"stable_sort": {
		Name:       "ahoy_dict_stable_sort",
		Category:   "dict",
		MethodName: "stable_sort",
		ReturnType: "dict",
		Params:     "d:dict",
		Doc:        "Stable sorts the dictionary by keys",
		Code: `HashMap* ahoy_dict_stable_sort(HashMap* dict) {
    return ahoy_dict_sort(dict);
}
`,
	},
	"merge": {
		Name:       "ahoy_dict_merge",
		Category:   "dict",
		MethodName: "merge",
		ReturnType: "dict",
		Params:     "d:dict, other:dict",
		Doc:        "Merges another dictionary into this one",
		Code: `HashMap* ahoy_dict_merge(HashMap* dict1, HashMap* dict2) {
    HashMap* merged = hashmap_create();
    for (int i = 0; i < dict1->capacity; i++) {
        if (dict1->entries[i].key) {
            hashmap_set(merged, dict1->entries[i].key, dict1->entries[i].value, dict1->entries[i].type);
        }
    }
    for (int i = 0; i < dict2->capacity; i++) {
        if (dict2->entries[i].key) {
            hashmap_set(merged, dict2->entries[i].key, dict2->entries[i].value, dict2->entries[i].type);
        }
    }
    return merged;
}
`,
	},
}

// StringMethods contains all string method implementations
var StringMethods = map[string]StdlibFunc{
	"dup": {
		Name:       "ahoy_string_dup",
		Category:   "string",
		MethodName: "dup",
		ReturnType: "string",
		Params:     "s:string",
		Doc:        "Duplicates a string (internal helper)",
		Code: `char* ahoy_string_dup(const char* src) {
    if (!src) return NULL;
    size_t len = strlen(src) + 1;
    char* dst = malloc(len);
    memcpy(dst, src, len);
    return dst;
}
`,
	},
	"length": {
		Name:       "ahoy_string_length",
		Category:   "string",
		MethodName: "length",
		ReturnType: "int",
		Params:     "s:string",
		Doc:        "Returns the length of the string",
		Code: `int ahoy_string_length(const char* str) {
    return str ? strlen(str) : 0;
}
`,
	},
	"upper": {
		Name:       "ahoy_string_upper",
		Category:   "string",
		MethodName: "upper",
		ReturnType: "string",
		Params:     "s:string",
		Doc:        "Converts the string to uppercase",
		Code: `char* ahoy_string_upper(const char* str) {
    if (!str) return NULL;
    char* result = ahoy_string_dup(str);
    for (int i = 0; result[i]; i++) {
        result[i] = toupper(result[i]);
    }
    return result;
}
`,
	},
	"lower": {
		Name:       "ahoy_string_lower",
		Category:   "string",
		MethodName: "lower",
		ReturnType: "string",
		Params:     "s:string",
		Doc:        "Converts the string to lowercase",
		Code: `char* ahoy_string_lower(const char* str) {
    if (!str) return NULL;
    char* result = ahoy_string_dup(str);
    for (int i = 0; result[i]; i++) {
        result[i] = tolower(result[i]);
    }
    return result;
}
`,
	},
	"replace": {
		Name:       "ahoy_string_replace",
		Category:   "string",
		MethodName: "replace",
		ReturnType: "string",
		Params:     "s:string, old:string, new:string",
		Doc:        "Replaces all occurrences of old with new",
		Code: `char* ahoy_string_replace(const char* str, const char* old, const char* new_str) {
    if (!str || !old || !new_str) return ahoy_string_dup(str);
    int old_len = strlen(old);
    int new_len = strlen(new_str);
    int count = 0;
    const char* tmp = str;
    while ((tmp = strstr(tmp, old))) { count++; tmp += old_len; }
    if (count == 0) return ahoy_string_dup(str);
    int result_len = strlen(str) + count * (new_len - old_len) + 1;
    char* result = malloc(result_len);
    char* dst = result;
    while (*str) {
        if (strstr(str, old) == str) {
            memcpy(dst, new_str, new_len);
            dst += new_len;
            str += old_len;
        } else {
            *dst++ = *str++;
        }
    }
    *dst = '\0';
    return result;
}
`,
	},
	"contains": {
		Name:       "ahoy_string_contains",
		Category:   "string",
		MethodName: "contains",
		ReturnType: "bool",
		Params:     "s:string, substr:string",
		Doc:        "Checks if the string contains a substring",
		Code: `bool ahoy_string_contains(const char* str, const char* substr) {
    return str && substr && strstr(str, substr) != NULL;
}
`,
	},
	"strip": {
		Name:       "ahoy_string_strip",
		Category:   "string",
		MethodName: "strip",
		ReturnType: "string",
		Params:     "s:string",
		Doc:        "Removes leading and trailing whitespace",
		Code: `char* ahoy_string_strip(const char* str) {
    if (!str) return NULL;
    if (!*str) return ahoy_string_dup("");
    while (*str && isspace(*str)) str++;
    const char* end = str + strlen(str) - 1;
    while (end > str && isspace(*end)) end--;
    int len = end - str + 1;
    char* result = malloc(len + 1);
    memcpy(result, str, len);
    result[len] = '\0';
    return result;
}
`,
	},
	"count": {
		Name:       "ahoy_string_count",
		Category:   "string",
		MethodName: "count",
		ReturnType: "int",
		Params:     "s:string, substr:string",
		Doc:        "Counts occurrences of a substring",
		Code: `int ahoy_string_count(const char* str, const char* substr) {
    if (!str || !substr || !*substr) return 0;
    int count = 0;
    int substr_len = strlen(substr);
    const char* pos = str;
    while ((pos = strstr(pos, substr))) {
        count++;
        pos += substr_len;
    }
    return count;
}
`,
	},
	"lpad": {
		Name:       "ahoy_string_lpad",
		Category:   "string",
		MethodName: "lpad",
		ReturnType: "string",
		Params:     "s:string, length:int, pad:string",
		Doc:        "Left-pads the string to the specified length",
		Code: `char* ahoy_string_lpad(const char* str, int length, const char* pad) {
    if (!str || !pad) return ahoy_string_dup(str);
    int str_len = strlen(str);
    if (str_len >= length) return ahoy_string_dup(str);
    int pad_len = strlen(pad);
    char* result = malloc(length + 1);
    int fill = length - str_len;
    int i = 0;
    while (i < fill) {
        result[i] = pad[i % pad_len];
        i++;
    }
    memcpy(result + fill, str, str_len + 1);
    return result;
}
`,
	},
	"rpad": {
		Name:       "ahoy_string_rpad",
		Category:   "string",
		MethodName: "rpad",
		ReturnType: "string",
		Params:     "s:string, length:int, pad:string",
		Doc:        "Right-pads the string to the specified length",
		Code: `char* ahoy_string_rpad(const char* str, int length, const char* pad) {
    if (!str || !pad) return ahoy_string_dup(str);
    int str_len = strlen(str);
    if (str_len >= length) return ahoy_string_dup(str);
    int pad_len = strlen(pad);
    char* result = malloc(length + 1);
    memcpy(result, str, str_len);
    int fill = length - str_len;
    for (int i = 0; i < fill; i++) {
        result[str_len + i] = pad[i % pad_len];
    }
    result[length] = '\0';
    return result;
}
`,
	},
	"pad": {
		Name:       "ahoy_string_pad",
		Category:   "string",
		MethodName: "pad",
		ReturnType: "string",
		Params:     "s:string, length:int, pad:string",
		Doc:        "Pads the string on both sides to the specified length",
		Code: `char* ahoy_string_pad(const char* str, int length, const char* pad) {
    if (!str || !pad) return ahoy_string_dup(str);
    int str_len = strlen(str);
    if (str_len >= length) return ahoy_string_dup(str);
    int pad_len = strlen(pad);
    int total_pad = length - str_len;
    int left_pad = total_pad / 2;
    int right_pad = total_pad - left_pad;
    char* result = malloc(length + 1);
    for (int i = 0; i < left_pad; i++) {
        result[i] = pad[i % pad_len];
    }
    memcpy(result + left_pad, str, str_len);
    for (int i = 0; i < right_pad; i++) {
        result[left_pad + str_len + i] = pad[i % pad_len];
    }
    result[length] = '\0';
    return result;
}
`,
	},
	"match": {
		Name:       "ahoy_string_match",
		Category:   "string",
		MethodName: "match",
		ReturnType: "bool",
		Params:     "s:string, pattern:string",
		Doc:        "Checks if the string matches a regex pattern",
		Code: `bool ahoy_string_match(const char* str, const char* pattern) {
    if (!str || !pattern) return false;
    regex_t regex;
    if (regcomp(&regex, pattern, REG_EXTENDED) != 0) return false;
    int result = regexec(&regex, str, 0, NULL, 0);
    regfree(&regex);
    return result == 0;
}
`,
	},
	"get_file": {
		Name:       "ahoy_string_get_file",
		Category:   "string",
		MethodName: "get_file",
		ReturnType: "string",
		Params:     "s:string",
		Doc:        "Extracts the filename from a file path",
		Code: `char* ahoy_string_get_file(const char* path) {
    if (!path) return NULL;
    char* filename = strrchr(path, '/');
    if (!filename) return ahoy_string_dup(path);
    return ahoy_string_dup(filename + 1);
}
`,
	},
	"camel_case": {
		Name:       "ahoy_string_camel_case",
		Category:   "string",
		MethodName: "camel_case",
		ReturnType: "string",
		Params:     "s:string",
		Doc:        "Converts the string to camelCase",
		Code: `char* ahoy_string_camel_case(const char* str) {
    if (!str) return NULL;
    char* result = malloc(strlen(str) + 1);
    int j = 0;
    int capitalize_next = 0;
    int first = 1;
    for (int i = 0; str[i]; i++) {
        if (str[i] == ' ' || str[i] == '_' || str[i] == '-') {
            capitalize_next = 1;
        } else if (capitalize_next) {
            result[j++] = toupper(str[i]);
            capitalize_next = 0;
        } else if (first) {
            result[j++] = tolower(str[i]);
            first = 0;
        } else {
            result[j++] = str[i];
        }
    }
    result[j] = '\0';
    return result;
}
`,
	},
	"snake_case": {
		Name:       "ahoy_string_snake_case",
		Category:   "string",
		MethodName: "snake_case",
		ReturnType: "string",
		Params:     "s:string",
		Doc:        "Converts the string to snake_case",
		Code: `char* ahoy_string_snake_case(const char* str) {
    if (!str) return NULL;
    char* result = malloc(strlen(str) * 2 + 1);
    int j = 0;
    for (int i = 0; str[i]; i++) {
        if (isupper(str[i]) && i > 0) {
            result[j++] = '_';
        }
        if (str[i] == ' ' || str[i] == '-') {
            result[j++] = '_';
        } else {
            result[j++] = tolower(str[i]);
        }
    }
    result[j] = '\0';
    return result;
}
`,
	},
	"pascal_case": {
		Name:       "ahoy_string_pascal_case",
		Category:   "string",
		MethodName: "pascal_case",
		ReturnType: "string",
		Params:     "s:string",
		Doc:        "Converts the string to PascalCase",
		Code: `char* ahoy_string_pascal_case(const char* str) {
    if (!str) return NULL;
    char* result = malloc(strlen(str) + 1);
    int j = 0;
    int capitalize_next = 1;
    for (int i = 0; str[i]; i++) {
        if (str[i] == ' ' || str[i] == '_' || str[i] == '-') {
            capitalize_next = 1;
        } else if (capitalize_next) {
            result[j++] = toupper(str[i]);
            capitalize_next = 0;
        } else {
            result[j++] = str[i];
        }
    }
    result[j] = '\0';
    return result;
}
`,
	},
	"kebab_case": {
		Name:       "ahoy_string_kebab_case",
		Category:   "string",
		MethodName: "kebab_case",
		ReturnType: "string",
		Params:     "s:string",
		Doc:        "Converts the string to kebab-case",
		Code: `char* ahoy_string_kebab_case(const char* str) {
    if (!str) return NULL;
    char* result = malloc(strlen(str) * 2 + 1);
    int j = 0;
    for (int i = 0; str[i]; i++) {
        if (isupper(str[i]) && i > 0) {
            result[j++] = '-';
        }
        if (str[i] == ' ' || str[i] == '_') {
            result[j++] = '-';
        } else {
            result[j++] = tolower(str[i]);
        }
    }
    result[j] = '\0';
    return result;
}
`,
	},
	"split": {
		Name:       "ahoy_string_split",
		Category:   "string",
		MethodName: "split",
		ReturnType: "array[string]",
		Params:     "s:string, delimiter:string",
		Doc:        "Splits the string by delimiter into an array",
		Code: `char** ahoy_string_split(const char* str, const char* delim) {
    if (!str || !delim) return NULL;
    int count = 1;
    const char* tmp = str;
    int delim_len = strlen(delim);
    while ((tmp = strstr(tmp, delim))) { count++; tmp += delim_len; }
    char** result = malloc((count + 1) * sizeof(char*));
    char* copy = ahoy_string_dup(str);
    char* token = copy;
    int i = 0;
    while (token) {
        char* next = strstr(token, delim);
        if (next) {
            *next = '\0';
            result[i++] = ahoy_string_dup(token);
            token = next + delim_len;
        } else {
            result[i++] = ahoy_string_dup(token);
            break;
        }
    }
    result[i] = NULL;
    free(copy);
    return result;
}
`,
	},
}

// BuiltinFuncs contains built-in function definitions (for documentation)
var BuiltinFuncs = map[string]StdlibFunc{
	"print": {
		Name:       "print",
		Category:   "builtin",
		MethodName: "print",
		ReturnType: "void",
		Params:     "value:any",
		Doc:        "Prints a value to stdout",
		Code:       "", // Built into compiler
	},
	"log": {
		Name:       "log",
		Category:   "builtin",
		MethodName: "log",
		ReturnType: "void",
		Params:     "value:any",
		Doc:        "Logs a value with debug formatting",
		Code:       "", // Built into compiler
	},
	"panic": {
		Name:       "panic",
		Category:   "builtin",
		MethodName: "panic",
		ReturnType: "void",
		Params:     "message:string",
		Doc:        "Terminates the program with an error message",
		Code:       "", // Built into compiler
	},
	"assert": {
		Name:       "assert",
		Category:   "builtin",
		MethodName: "assert",
		ReturnType: "void",
		Params:     "condition:bool, message:string",
		Doc:        "Asserts a condition is true, panics otherwise",
		Code:       "", // Built into compiler
	},
	"free": {
		Name:       "free",
		Category:   "builtin",
		MethodName: "free",
		ReturnType: "void",
		Params:     "ptr:any",
		Doc:        "Frees heap-allocated memory",
		Code:       "", // Built into compiler
	},
}

// GetAllStdlibFuncs returns all stdlib functions for documentation generation
func GetAllStdlibFuncs() []StdlibFunc {
	var all []StdlibFunc
	for _, f := range ArrayMethods {
		all = append(all, f)
	}
	for _, f := range DictMethods {
		all = append(all, f)
	}
	for _, f := range StringMethods {
		all = append(all, f)
	}
	for _, f := range BuiltinFuncs {
		all = append(all, f)
	}
	return all
}
