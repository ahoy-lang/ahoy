

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
```
