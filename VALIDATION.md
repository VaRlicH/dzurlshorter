# Результаты проверки


```text
go test -coverprofile=coverage.out ./...
ok  urlshortener  1.808s  coverage: 92.6% of statements

go tool cover -func=coverage.out
newHandler        100.0%
shortenHandler    100.0%
writeError        100.0%
writeJSON         100.0%
main                0.0%
NewURLShortener   100.0%
Shorten           100.0%
GetOriginal       100.0%
generateShortID    75.0%
isValidURL        100.0%
total             92.6%

go test -race ./...
ok  urlshortener  2.507s

go vet ./...
(замечаний нет)
```



