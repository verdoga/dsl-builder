# Console и main: результаты проверки

Изменение добавляет последовательный русский диалог [console](../../console/doc.go)
и [точку входа](../../readme.md#сборка-и-запуск). Принятые исходники,
тесты и входные fixtures перенесены побайтно; правила [ТЗ](../../TZ.md), схемы и
original golden не изменялись. База: `7412efa441e9b6bbf7ec95e04452965a8b34e34b`.

В состав входят console (13 production, 81 test, 7 fixtures, включая корневой
`testdata/run/source.json`), main (2 production, 7 test, 4 synthetic fixtures,
2 README), одна ячейка графа в `packages.md` и разрешённый DEP-Q01: прямой
`filetree.Resolve` в `app.SetDestination`, четыре регрессионных файла и две
коррекции аргументов в существующих тестах. Всего 124 принятых файла и эта сводка.
Сборочные инструкции передают версию через `-X main.buildVersion=...`.

Этот документ сохраняет исторические результаты проверки до переноса точки
входа в корень проекта. В приведённых ниже командах `./cmd/dsl-builder` —
прежний путь пакета `main`; для текущей структуры используется `.`.

## Новая интеграционная проверка

Go 1.25.14, Linux amd64, стандартная библиотека, исходники без изменений.

| Проверка | Результат |
| --- | --- |
| `go test -json -count=1 ./...` | **FAIL**: 25/26 пакетов PASS; только прежний filetree baseline |
| `go test -race -json -count=1 ./console ./cmd/dsl-builder ./app` | PASS: 508 top-level / 1694 terminal test events, без fail/skip |
| `go vet ./...`, Linux `go build ./...`, read-only `gofmt -l` | PASS |
| Windows amd64 CGO0 `go build ./...` и test compile console/main/app | PASS, cross compilation |
| Обычный Linux binary с `-X main.buildVersion=v1.0.0`: numeric exit, EOF, args notice | PASS: exit0, верный заголовок, stderr пуст |

Общий прогон: 12022 PASS / 82 FAIL / 1 skip test events; 1738 PASS / 9 FAIL /
1 skip top-level. Все 82 failed identities и единственный UNC skip точно совпали
с принятым baseline. Тесты filetree не исключались и не ослаблялись. Общий PASS
проекта не заявляется. Native Windows и double-click не запускались; согласованный
объём — Linux behavior плюс Windows cross.

## Принятые пакеты и отдельные production-наблюдения

Senior console43 и main11 дали положительную техническую и отдельную функциональную
приёмку. Это исторические решения о замороженном составе, отдельно от следующих
реальных production-прогонов принятого Linux binary `v1.0.0`.

| Последующий прогон | Наблюдаемый результат |
| --- | --- |
| Все 7 original parsed → настоящий CLI → original golden, строго без нормализаций | ZIP 5/7; strict match **0/7**. Cases 1/3: `resourcefind.not_found`, результата нет. В пяти созданных lesson различается `createdAt`; cases 2/5 также не содержат ожидаемый `multifill.flags.checkable: false`. |
| Отдельные media variants 1/3 с реальными MP3/MP4, TXT/VTT | ZIP 2/2; все 6 встроенных файлов bytes-exact. При исключении **только** `/document/createdAt` strict lesson **FAIL 2/2**: отсутствует `flags.checkable: false` у `efx7-n003` и `ba83-n018`. |

В original cases 2/5 отсутствующие поля находятся соответственно по JSON Pointer
`/document/pages/0/nodes/0/nodes/3/flags/checkable` и
`/document/pages/1/nodes/0/nodes/1/flags/checkable`. Missing не приравнивался к false
или null. Для original-прогона дата не исключалась. Для media variants были заранее
заморожены отдельные копии parsed/expected и реальные ресурсы; original golden
не менялись. Эти наблюдения по прямому указанию пользователя завершены только
отчётом: код, тесты и эталоны по ним не исправлялись. Media variants, результаты
запусков, логи, PCM и binaries в этот PR не входят.

## Provenance

Идентификаторы исходных локальных отчётов и SHA256 (полные архивы evidence не
публикуются в этом PR):

| Принятый документ | SHA256 |
| --- | --- |
| console Senior43 | `01d7dcd88246331011c12217faea1114c87b5154b4979efa12c955fef0e61333` |
| console final-freeze | `e802eb97fb72da3e0d84572017331fcf7999d1382d37f3195ebf72f4a772366a` |
| main Senior11 | `41045272b015f1aa549ce7e064bf2c82b1c848f3a6f06b718c5074af13c5b00e` |
| main final-freeze | `6db130d328157aa101259b71b64edb4c8c962b5d20c4726000f65f8d0604a780` |
| original production12 | `60ebee24f0dab5e93d769c84c6b981f93d175b077d1ecfbb39f3c188c7a2aab8` |
| media fixture-freeze13 | `18ee45e720250cda06603e86532b35d75722b27e11935397e636d4b6f0845573` |
| media production14 | `ad1770591434cdd9960319348f052684128b5dda150350e9f90aaff9511349f6` |

Принятые release artifacts сохранены вне Git: Linux SHA256
`b7fcd82594e5bcb18d4f552e65279d36bf2b3ce16c21a03472164b9321b3512b`,
Windows SHA256 `f3ab685ec3c9ad5dabdbd8a3884cac22ad509a38ad34f4a2c9653e12d6cfc3f6`.
Они не заменялись проверочными сборками и не включены в commit.
