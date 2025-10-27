
# Python-Like Dictionaries

Hash maps with Python-style syntax.

**Syntax:**
```ahoy
# Declaration
config: {"name":"PyLang", "version":2, "active":true}

# Access with curly braces
app_name: config{"name"}
version_num: config{"version"}
is_active: config{"active"}
```

**Features:**
- String keys
- Mixed value types
- HashMap implementation in C
