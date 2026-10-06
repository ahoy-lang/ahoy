
```ahoy

? example of tuple assignment in ahoy
  a : 5
  b : 10
  a, b : b, a  ? Swaps the values of a and b
  ? Now, a is 10 and b is 5

  my_func :: || int,int,string:
			return 1, 2, "three"
  $
  one , two, three : my_func||  ? Unpacks the returned tuple into variables
  ? the explicit return types make one and two int, and three string
  print|f"One: {one}, Two: {two}, Three: {three}\n"|
```
