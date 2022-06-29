# CMD 1: read from a file descriptor

```
+-----+-----+
| Rsv | Len |
+-----+-----+
|  1  |  2  |
+-----+-----+
```

```
+-----+-----+-----+
| Rsv | Len | Msg |
+-----+-----+-----+
|  1  |  2  |  N  |
+-----+-----+-----+
```

# CMD 2: Write to a file descriptor

```
+-----+-----+-----+-----+
| Rsv | ID  | Len | Msg |
+-----+-----+-----+-----+
|  1  |  2  |  2  |  N  |
+-----+-----+-----+-----+
```

# CMD 3: open and possibly create a file

Client wishes to establish a connection.

```
+-----+-----+-----+
| Net | Len | Dst |
+-----+-----+-----+
|  1  |  2  |  N  |
+-----+-----+-----+
```

```
+-----+-----+
| Rep | ID  |
+-----+-----+
|  1  |  2  |
+-----+-----+
```

Rep: Reply field
     0 succeeded
     1 general failure

# CMD 4: close a file descriptor

```
+-----+-----+
| Rsv | ID  |
+-----+-----+
|  1  |  2  |
+-----+-----+
```
