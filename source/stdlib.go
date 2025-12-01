package main

// StdlibFunc represents a stdlib function definition
type StdlibFunc struct {
	Name       string
	Category   string
	MethodName string
	ReturnType string
	Params     string
	Doc        string
	Code       string
}

// ArrayMethods contains all array method implementations
var ArrayMethods = map[string]StdlibFunc{
	"length": {
		Name: "ahoy_array_length", Category: "array", MethodName: "length", ReturnType: "int",
		Params: "arr:array", Doc: "Returns the number of elements in the array",
		Code: "int ahoy_array_length(AhoyArray* arr) {\n    return arr->length;\n}\n",
	},
	"push": {
		Name: "ahoy_array_push", Category: "array", MethodName: "push", ReturnType: "array",
		Params: "arr:array, value:any", Doc: "Adds an element to the end of the array",
		Code: "AhoyArray* ahoy_array_push(AhoyArray* arr, intptr_t value, AhoyValueType type) {\n    if (arr->length >= arr->capacity) {\n        arr->capacity = arr->capacity == 0 ? 4 : arr->capacity * 2;\n        arr->data = realloc(arr->data, arr->capacity * sizeof(intptr_t));\n        arr->types = realloc(arr->types, arr->capacity * sizeof(AhoyValueType));\n    }\n    arr->data[arr->length] = value;\n    arr->types[arr->length] = type;\n    arr->length++;\n    return arr;\n}\n",
	},
	"pop": {
		Name: "ahoy_array_pop", Category: "array", MethodName: "pop", ReturnType: "any",
		Params: "arr:array", Doc: "Removes and returns the last element",
		Code: "intptr_t ahoy_array_pop(AhoyArray* arr) {\n    if (arr->length == 0) return 0;\n    return arr->data[--arr->length];\n}\n",
	},
	"sum": {
		Name: "ahoy_array_sum", Category: "array", MethodName: "sum", ReturnType: "int",
		Params: "arr:array", Doc: "Returns the sum of all numeric elements",
		Code: "int ahoy_array_sum(AhoyArray* arr) {\n    int total = 0;\n    for (int i = 0; i < arr->length; i++) {\n        total += (int)arr->data[i];\n    }\n    return total;\n}\n",
	},
	"has": {
		Name: "ahoy_array_has", Category: "array", MethodName: "has", ReturnType: "bool",
		Params: "arr:array, value:any", Doc: "Checks if the array contains a specific element",
		Code: "int ahoy_array_has(AhoyArray* arr, intptr_t value) {\n    for (int i = 0; i < arr->length; i++) {\n        if (arr->data[i] == value) return 1;\n    }\n    return 0;\n}\n",
	},
	"sort": {
		Name: "ahoy_array_sort", Category: "array", MethodName: "sort", ReturnType: "array",
		Params: "arr:array", Doc: "Sorts the array in ascending order",
		Code: "int __ahoy_compare_int(const void* a, const void* b) {\n    return (*(intptr_t*)a - *(intptr_t*)b);\n}\n\nAhoyArray* ahoy_array_sort(AhoyArray* arr) {\n    qsort(arr->data, arr->length, sizeof(intptr_t), __ahoy_compare_int);\n    return arr;\n}\n",
	},
	"reverse": {
		Name: "ahoy_array_reverse", Category: "array", MethodName: "reverse", ReturnType: "array",
		Params: "arr:array", Doc: "Reverses the array in place",
		Code: "AhoyArray* ahoy_array_reverse(AhoyArray* arr) {\n    for (int i = 0; i < arr->length / 2; i++) {\n        intptr_t temp = arr->data[i];\n        arr->data[i] = arr->data[arr->length - 1 - i];\n        arr->data[arr->length - 1 - i] = temp;\n    }\n    return arr;\n}\n",
	},
	"shuffle": {
		Name: "ahoy_array_shuffle", Category: "array", MethodName: "shuffle", ReturnType: "array",
		Params: "arr:array", Doc: "Randomly shuffles the array",
		Code: "AhoyArray* ahoy_array_shuffle(AhoyArray* arr) {\n    srand(time(NULL));\n    for (int i = arr->length - 1; i > 0; i--) {\n        int j = rand() % (i + 1);\n        intptr_t temp = arr->data[i];\n        arr->data[i] = arr->data[j];\n        arr->data[j] = temp;\n    }\n    return arr;\n}\n",
	},
	"pick": {
		Name: "ahoy_array_pick", Category: "array", MethodName: "pick", ReturnType: "any",
		Params: "arr:array", Doc: "Returns a random element from the array",
		Code: "intptr_t ahoy_array_pick(AhoyArray* arr) {\n    if (arr->length == 0) return 0;\n    srand(time(NULL));\n    return arr->data[rand() % arr->length];\n}\n",
	},
	"fill": {
		Name: "ahoy_array_fill", Category: "array", MethodName: "fill", ReturnType: "array",
		Params: "arr:array, value:any, count:int", Doc: "Fills the array with the specified value",
		Code: "AhoyArray* ahoy_array_fill(AhoyArray* arr, intptr_t value, AhoyValueType type, int count) {\n    if (count <= 0) return arr;\n    if (arr->capacity < count) {\n        arr->capacity = count;\n        arr->data = realloc(arr->data, arr->capacity * sizeof(intptr_t));\n        arr->types = realloc(arr->types, arr->capacity * sizeof(AhoyValueType));\n    }\n    for (int i = 0; i < count; i++) {\n        arr->data[i] = value;\n        arr->types[i] = type;\n    }\n    arr->length = count;\n    return arr;\n}\n",
	},
}

// DictMethods contains all dictionary method implementations
var DictMethods = map[string]StdlibFunc{
	"size": {
		Name: "ahoy_dict_size", Category: "dict", MethodName: "size", ReturnType: "int",
		Params: "d:dict", Doc: "Returns the number of key-value pairs",
		Code: "int ahoy_dict_size(HashMap* dict) {\n    if (dict == NULL) return 0;\n    return dict->size;\n}\n",
	},
	"clear": {
		Name: "ahoy_dict_clear", Category: "dict", MethodName: "clear", ReturnType: "void",
		Params: "d:dict", Doc: "Removes all entries from the dictionary",
		Code: "void ahoy_dict_clear(HashMap* dict) {\n    if (dict == NULL) return;\n    for (int i = 0; i < dict->capacity; i++) {\n        HashMapEntry* entry = dict->buckets[i];\n        while (entry != NULL) {\n            HashMapEntry* temp = entry;\n            entry = entry->next;\n            free(temp->key);\n            free(temp);\n        }\n        dict->buckets[i] = NULL;\n    }\n    dict->size = 0;\n}\n",
	},
	"has": {
		Name: "ahoy_dict_has", Category: "dict", MethodName: "has", ReturnType: "bool",
		Params: "d:dict, key:string", Doc: "Checks if a key exists",
		Code: "int ahoy_dict_has(HashMap* dict, char* key) {\n    if (dict == NULL || key == NULL) return 0;\n    return hashMapGet(dict, key) != NULL ? 1 : 0;\n}\n",
	},
	"has_all": {
		Name: "ahoy_dict_has_all", Category: "dict", MethodName: "has_all", ReturnType: "bool",
		Params: "d:dict, keys:array[string]", Doc: "Checks if all keys exist",
		Code: "int ahoy_dict_has_all(HashMap* dict, AhoyArray* keys) {\n    if (dict == NULL || keys == NULL) return 0;\n    for (int i = 0; i < keys->length; i++) {\n        char* key = (char*)(intptr_t)keys->data[i];\n        if (hashMapGet(dict, key) == NULL) return 0;\n    }\n    return 1;\n}\n",
	},
	"keys": {
		Name: "ahoy_dict_keys", Category: "dict", MethodName: "keys", ReturnType: "array[string]",
		Params: "d:dict", Doc: "Returns all keys",
		Code: "AhoyArray* ahoy_dict_keys(HashMap* dict) {\n    AhoyArray* arr = malloc(sizeof(AhoyArray));\n    arr->length = 0;\n    arr->capacity = dict->size;\n    arr->data = malloc(arr->capacity * sizeof(int));\n    for (int i = 0; i < dict->capacity; i++) {\n        HashMapEntry* entry = dict->buckets[i];\n        while (entry != NULL) {\n            arr->data[arr->length++] = (int)(intptr_t)entry->key;\n            entry = entry->next;\n        }\n    }\n    return arr;\n}\n",
	},
	"values": {
		Name: "ahoy_dict_values", Category: "dict", MethodName: "values", ReturnType: "array",
		Params: "d:dict", Doc: "Returns all values",
		Code: "AhoyArray* ahoy_dict_values(HashMap* dict) {\n    AhoyArray* arr = malloc(sizeof(AhoyArray));\n    arr->length = 0;\n    arr->capacity = dict->size;\n    arr->data = malloc(arr->capacity * sizeof(int));\n    for (int i = 0; i < dict->capacity; i++) {\n        HashMapEntry* entry = dict->buckets[i];\n        while (entry != NULL) {\n            arr->data[arr->length++] = (int)(intptr_t)entry->value;\n            entry = entry->next;\n        }\n    }\n    return arr;\n}\n",
	},
	"sort": {
		Name: "ahoy_dict_sort", Category: "dict", MethodName: "sort", ReturnType: "dict",
		Params: "d:dict", Doc: "Sorts dictionary by keys",
		Code: "int __ahoy_compare_keys(const void* a, const void* b) {\n    return strcmp((char*)a, (char*)b);\n}\n\nHashMap* ahoy_dict_sort(HashMap* dict) {\n    if (dict == NULL || dict->size == 0) return dict;\n    char** keys = malloc(dict->size * sizeof(char*));\n    int idx = 0;\n    for (int i = 0; i < dict->capacity; i++) {\n        HashMapEntry* entry = dict->buckets[i];\n        while (entry != NULL) {\n            keys[idx++] = entry->key;\n            entry = entry->next;\n        }\n    }\n    qsort(keys, dict->size, sizeof(char*), __ahoy_compare_keys);\n    HashMap* sorted = createHashMap(dict->capacity);\n    for (int i = 0; i < dict->size; i++) {\n        void* value = hashMapGet(dict, keys[i]);\n        hashMapPut(sorted, keys[i], value);\n    }\n    free(keys);\n    return sorted;\n}\n",
	},
	"stable_sort": {
		Name: "ahoy_dict_stable_sort", Category: "dict", MethodName: "stable_sort", ReturnType: "dict",
		Params: "d:dict", Doc: "Stable sorts dictionary by keys",
		Code: "HashMap* ahoy_dict_stable_sort(HashMap* dict) {\n    return ahoy_dict_sort(dict);\n}\n",
	},
	"merge": {
		Name: "ahoy_dict_merge", Category: "dict", MethodName: "merge", ReturnType: "dict",
		Params: "d:dict, other:dict", Doc: "Merges two dictionaries",
		Code: "HashMap* ahoy_dict_merge(HashMap* dict1, HashMap* dict2) {\n    if (dict1 == NULL) return dict2;\n    if (dict2 == NULL) return dict1;\n    HashMap* merged = createHashMap(dict1->capacity + dict2->capacity);\n    for (int i = 0; i < dict1->capacity; i++) {\n        HashMapEntry* entry = dict1->buckets[i];\n        while (entry != NULL) {\n            hashMapPut(merged, entry->key, entry->value);\n            entry = entry->next;\n        }\n    }\n    for (int i = 0; i < dict2->capacity; i++) {\n        HashMapEntry* entry = dict2->buckets[i];\n        while (entry != NULL) {\n            hashMapPut(merged, entry->key, entry->value);\n            entry = entry->next;\n        }\n    }\n    return merged;\n}\n",
	},
}

// StringMethods contains all string method implementations
var StringMethods = map[string]StdlibFunc{
	"dup": {
		Name: "ahoy_string_dup", Category: "string", MethodName: "dup", ReturnType: "string",
		Params: "s:string", Doc: "Duplicates a string",
		Code: "char* ahoy_string_dup(const char* src) {\n    if (!src) return NULL;\n    char* dest = malloc(strlen(src) + 1);\n    strcpy(dest, src);\n    return dest;\n}\n",
	},
	"length": {
		Name: "ahoy_string_length", Category: "string", MethodName: "length", ReturnType: "int",
		Params: "s:string", Doc: "Returns string length",
		Code: "int ahoy_string_length(const char* str) {\n    return str ? strlen(str) : 0;\n}\n",
	},
	"upper": {
		Name: "ahoy_string_upper", Category: "string", MethodName: "upper", ReturnType: "string",
		Params: "s:string", Doc: "Converts to uppercase",
		Code: "char* ahoy_string_upper(const char* str) {\n    if (!str) return NULL;\n    char* result = ahoy_string_dup(str);\n    for (int i = 0; result[i]; i++) {\n        result[i] = toupper(result[i]);\n    }\n    return result;\n}\n",
	},
	"lower": {
		Name: "ahoy_string_lower", Category: "string", MethodName: "lower", ReturnType: "string",
		Params: "s:string", Doc: "Converts to lowercase",
		Code: "char* ahoy_string_lower(const char* str) {\n    if (!str) return NULL;\n    char* result = ahoy_string_dup(str);\n    for (int i = 0; result[i]; i++) {\n        result[i] = tolower(result[i]);\n    }\n    return result;\n}\n",
	},
	"replace": {
		Name: "ahoy_string_replace", Category: "string", MethodName: "replace", ReturnType: "string",
		Params: "s:string, old:string, new:string", Doc: "Replaces occurrences",
		Code: "char* ahoy_string_replace(const char* str, const char* old, const char* new_str) {\n    if (!str || !old || !new_str) return ahoy_string_dup(str);\n    int count = 0;\n    const char* tmp = str;\n    while ((tmp = strstr(tmp, old))) {\n        count++;\n        tmp += strlen(old);\n    }\n    int old_len = strlen(old);\n    int new_len = strlen(new_str);\n    int result_len = strlen(str) + count * (new_len - old_len);\n    char* result = malloc(result_len + 1);\n    char* ptr = result;\n    while (*str) {\n        if (strstr(str, old) == str) {\n            strcpy(ptr, new_str);\n            ptr += new_len;\n            str += old_len;\n        } else {\n            *ptr++ = *str++;\n        }\n    }\n    *ptr = '\\0';\n    return result;\n}\n",
	},
	"contains": {
		Name: "ahoy_string_contains", Category: "string", MethodName: "contains", ReturnType: "bool",
		Params: "s:string, substr:string", Doc: "Checks for substring",
		Code: "bool ahoy_string_contains(const char* str, const char* substr) {\n    if (!str || !substr) return false;\n    return strstr(str, substr) != NULL;\n}\n",
	},
	"strip": {
		Name: "ahoy_string_strip", Category: "string", MethodName: "strip", ReturnType: "string",
		Params: "s:string", Doc: "Removes whitespace",
		Code: "char* ahoy_string_strip(const char* str) {\n    if (!str) return NULL;\n    while (*str && isspace(*str)) str++;\n    if (!*str) return ahoy_string_dup(\"\");\n    const char* end = str + strlen(str) - 1;\n    while (end > str && isspace(*end)) end--;\n    int len = end - str + 1;\n    char* result = malloc(len + 1);\n    strncpy(result, str, len);\n    result[len] = '\\0';\n    return result;\n}\n",
	},
	"count": {
		Name: "ahoy_string_count", Category: "string", MethodName: "count", ReturnType: "int",
		Params: "s:string, substr:string", Doc: "Counts occurrences",
		Code: "int ahoy_string_count(const char* str, const char* substr) {\n    if (!str || !substr) return 0;\n    int count = 0;\n    const char* tmp = str;\n    while ((tmp = strstr(tmp, substr))) {\n        count++;\n        tmp += strlen(substr);\n    }\n    return count;\n}\n",
	},
	"lpad": {
		Name: "ahoy_string_lpad", Category: "string", MethodName: "lpad", ReturnType: "string",
		Params: "s:string, length:int, pad:string", Doc: "Left-pads string",
		Code: "char* ahoy_string_lpad(const char* str, int length, const char* pad) {\n    if (!str || !pad) return ahoy_string_dup(str);\n    int str_len = strlen(str);\n    if (str_len >= length) return ahoy_string_dup(str);\n    int pad_len = length - str_len;\n    char* result = malloc(length + 1);\n    int pad_char_len = strlen(pad);\n    for (int i = 0; i < pad_len; i++) {\n        result[i] = pad[i % pad_char_len];\n    }\n    strcpy(result + pad_len, str);\n    return result;\n}\n",
	},
	"rpad": {
		Name: "ahoy_string_rpad", Category: "string", MethodName: "rpad", ReturnType: "string",
		Params: "s:string, length:int, pad:string", Doc: "Right-pads string",
		Code: "char* ahoy_string_rpad(const char* str, int length, const char* pad) {\n    if (!str || !pad) return ahoy_string_dup(str);\n    int str_len = strlen(str);\n    if (str_len >= length) return ahoy_string_dup(str);\n    int pad_len = length - str_len;\n    char* result = malloc(length + 1);\n    strcpy(result, str);\n    int pad_char_len = strlen(pad);\n    for (int i = 0; i < pad_len; i++) {\n        result[str_len + i] = pad[i % pad_char_len];\n    }\n    result[length] = '\\0';\n    return result;\n}\n",
	},
	"pad": {
		Name: "ahoy_string_pad", Category: "string", MethodName: "pad", ReturnType: "string",
		Params: "s:string, length:int, pad:string", Doc: "Center-pads string",
		Code: "char* ahoy_string_pad(const char* str, int length, const char* pad) {\n    if (!str || !pad) return ahoy_string_dup(str);\n    int str_len = strlen(str);\n    if (str_len >= length) return ahoy_string_dup(str);\n    int total_pad = length - str_len;\n    int left_pad = total_pad / 2;\n    int right_pad = total_pad - left_pad;\n    char* result = malloc(length + 1);\n    int pad_char_len = strlen(pad);\n    for (int i = 0; i < left_pad; i++) {\n        result[i] = pad[i % pad_char_len];\n    }\n    strcpy(result + left_pad, str);\n    for (int i = 0; i < right_pad; i++) {\n        result[left_pad + str_len + i] = pad[i % pad_char_len];\n    }\n    result[length] = '\\0';\n    return result;\n}\n",
	},
	"match": {
		Name: "ahoy_string_match", Category: "string", MethodName: "match", ReturnType: "bool",
		Params: "s:string, pattern:string", Doc: "Regex match",
		Code: "bool ahoy_string_match(const char* str, const char* pattern) {\n    if (!str || !pattern) return false;\n    regex_t regex;\n    int ret = regcomp(&regex, pattern, REG_EXTENDED | REG_NOSUB);\n    if (ret) return false;\n    ret = regexec(&regex, str, 0, NULL, 0);\n    regfree(&regex);\n    return ret == 0;\n}\n",
	},
	"get_file": {
		Name: "ahoy_string_get_file", Category: "string", MethodName: "get_file", ReturnType: "string",
		Params: "path:string", Doc: "Gets filename from path",
		Code: "char* ahoy_string_get_file(const char* path) {\n    if (!path) return NULL;\n    const char* filename = strrchr(path, '/');\n    if (!filename) filename = strrchr(path, '\\\\');\n    if (!filename) return ahoy_string_dup(path);\n    return ahoy_string_dup(filename + 1);\n}\n",
	},
	"camel_case": {
		Name: "ahoy_string_camel_case", Category: "string", MethodName: "camel_case", ReturnType: "string",
		Params: "s:string", Doc: "Converts to camelCase",
		Code: "char* ahoy_string_camel_case(const char* str) {\n    if (!str) return NULL;\n    char* result = malloc(strlen(str) + 1);\n    int j = 0;\n    bool capitalize_next = false;\n    bool first = true;\n    for (int i = 0; str[i]; i++) {\n        if (str[i] == ' ' || str[i] == '_' || str[i] == '-') {\n            capitalize_next = true;\n        } else if (capitalize_next) {\n            result[j++] = toupper(str[i]);\n            capitalize_next = false;\n        } else if (first) {\n            result[j++] = tolower(str[i]);\n            first = false;\n        } else {\n            result[j++] = str[i];\n        }\n    }\n    result[j] = '\\0';\n    return result;\n}\n",
	},
	"snake_case": {
		Name: "ahoy_string_snake_case", Category: "string", MethodName: "snake_case", ReturnType: "string",
		Params: "s:string", Doc: "Converts to snake_case",
		Code: "char* ahoy_string_snake_case(const char* str) {\n    if (!str) return NULL;\n    char* result = malloc(strlen(str) * 2 + 1);\n    int j = 0;\n    for (int i = 0; str[i]; i++) {\n        if (str[i] == ' ' || str[i] == '-') {\n            result[j++] = '_';\n        } else if (isupper(str[i]) && i > 0) {\n            result[j++] = '_';\n            result[j++] = tolower(str[i]);\n        } else {\n            result[j++] = tolower(str[i]);\n        }\n    }\n    result[j] = '\\0';\n    return result;\n}\n",
	},
	"pascal_case": {
		Name: "ahoy_string_pascal_case", Category: "string", MethodName: "pascal_case", ReturnType: "string",
		Params: "s:string", Doc: "Converts to PascalCase",
		Code: "char* ahoy_string_pascal_case(const char* str) {\n    if (!str) return NULL;\n    char* result = malloc(strlen(str) + 1);\n    int j = 0;\n    bool capitalize_next = true;\n    for (int i = 0; str[i]; i++) {\n        if (str[i] == ' ' || str[i] == '_' || str[i] == '-') {\n            capitalize_next = true;\n        } else if (capitalize_next) {\n            result[j++] = toupper(str[i]);\n            capitalize_next = false;\n        } else {\n            result[j++] = tolower(str[i]);\n        }\n    }\n    result[j] = '\\0';\n    return result;\n}\n",
	},
	"kebab_case": {
		Name: "ahoy_string_kebab_case", Category: "string", MethodName: "kebab_case", ReturnType: "string",
		Params: "s:string", Doc: "Converts to kebab-case",
		Code: "char* ahoy_string_kebab_case(const char* str) {\n    if (!str) return NULL;\n    char* result = malloc(strlen(str) * 2 + 1);\n    int j = 0;\n    for (int i = 0; str[i]; i++) {\n        if (str[i] == ' ' || str[i] == '_') {\n            result[j++] = '-';\n        } else if (isupper(str[i]) && i > 0) {\n            result[j++] = '-';\n            result[j++] = tolower(str[i]);\n        } else {\n            result[j++] = tolower(str[i]);\n        }\n    }\n    result[j] = '\\0';\n    return result;\n}\n",
	},
	"split": {
		Name: "ahoy_string_split", Category: "string", MethodName: "split", ReturnType: "array[string]",
		Params: "s:string, delimiter:string", Doc: "Splits string by delimiter",
		Code: "char** ahoy_string_split(const char* str, const char* delim) {\n    if (!str || !delim) return NULL;\n    char* str_copy = ahoy_string_dup(str);\n    int count = 1;\n    for (const char* p = str; *p; p++) {\n        if (strstr(p, delim) == p) count++;\n    }\n    char** result = malloc((count + 1) * sizeof(char*));\n    char* token = strtok(str_copy, delim);\n    int i = 0;\n    while (token != NULL) {\n        result[i++] = ahoy_string_dup(token);\n        token = strtok(NULL, delim);\n    }\n    result[i] = NULL;\n    free(str_copy);\n    return result;\n}\n",
	},
}

// BuiltinFuncs contains built-in function definitions
var BuiltinFuncs = map[string]StdlibFunc{
	"print":      {Name: "print", Category: "builtin", MethodName: "print", ReturnType: "void", Params: "value:any", Doc: "Prints a value to stdout"},
	"log":        {Name: "log", Category: "builtin", MethodName: "log", ReturnType: "void", Params: "value:any", Doc: "Logs a value with debug formatting"},
	"panic":      {Name: "panic", Category: "builtin", MethodName: "panic", ReturnType: "void", Params: "message:string", Doc: "Terminates with error message"},
	"assert":     {Name: "assert", Category: "builtin", MethodName: "assert", ReturnType: "void", Params: "condition:bool, message:string", Doc: "Asserts condition is true"},
	"free":       {Name: "free", Category: "builtin", MethodName: "free", ReturnType: "void", Params: "ptr:any", Doc: "Frees heap-allocated memory"},
	"read_json":  {Name: "read_json", Category: "builtin", MethodName: "read_json", ReturnType: "json, string", Params: "filename:string", Doc: "Reads and parses a JSON file"},
	"write_json": {Name: "write_json", Category: "builtin", MethodName: "write_json", ReturnType: "string", Params: "filename:string, data:json", Doc: "Writes JSON to file"},
	"len":        {Name: "len", Category: "builtin", MethodName: "len", ReturnType: "int", Params: "value:any", Doc: "Returns length of arrays/strings/dicts"},
	"typeof":     {Name: "typeof", Category: "builtin", MethodName: "typeof", ReturnType: "string", Params: "value:any", Doc: "Returns type as string"},
	"str":        {Name: "str", Category: "builtin", MethodName: "str", ReturnType: "string", Params: "value:any", Doc: "Converts to string"},
	"int":        {Name: "int", Category: "builtin", MethodName: "int", ReturnType: "int", Params: "value:any", Doc: "Converts to integer"},
	"float":      {Name: "float", Category: "builtin", MethodName: "float", ReturnType: "float", Params: "value:any", Doc: "Converts to float"},
	"input":      {Name: "input", Category: "builtin", MethodName: "input", ReturnType: "string", Params: "prompt:string", Doc: "Reads input from stdin"},
	"sleep":      {Name: "sleep", Category: "builtin", MethodName: "sleep", ReturnType: "void", Params: "milliseconds:int", Doc: "Pauses execution"},
	"exit":       {Name: "exit", Category: "builtin", MethodName: "exit", ReturnType: "void", Params: "code:int", Doc: "Exits with code"},
	"rand":       {Name: "rand", Category: "builtin", MethodName: "rand", ReturnType: "int", Params: "", Doc: "Returns random integer"},
	"rand_range": {Name: "rand_range", Category: "builtin", MethodName: "rand_range", ReturnType: "int", Params: "min:int, max:int", Doc: "Returns random in range"},
}

// GetAllStdlibFuncs returns all stdlib functions
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
