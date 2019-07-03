### backoff component usage

- default usage try-times:100; timeinterval: random 0~16s

```golang
builder.AddComponent(new(backoff.Component))
```

- usage with attempt-times setting

```golang
backoffOptions := []backoff.ComponentOption{
    backoff.WithMaxAttempts(100),  //try again times;
}
builder.AddComponent(backoff.New(backoffOptions...))
```

### keepalive component usage
- default usage
```golang
    builder.AddComponent(new(keepalibe.Component))
```

- usage with options setting
```golang

```

### discovery component usage
- default usage
```golang

```
- usage with options setting
```golang

```

### proxy component usage
- default usage
```golang

```
- usage with options setting
```golang

```

### addressmap component usage
- default usage
```golang

```

- usage with options setting
```golang

```
