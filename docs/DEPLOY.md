# Развёртывание garage на relay

Как garage установлен на VPS `relay`, как его обновлять, восстанавливать из копии и что делать при сбоях. Требования — ТЗ, раздел 5 (Р5.1–Р5.9).

## Схема

```
Телефон / Mac ──VPN (AmneziaWG)──► relay
                                    │
            AdGuard: garage.north-flow.online → 10.211.7.1
                                    │
            nginx 10.211.7.1:443 (сертификат *.north-flow.online)
                                    │ proxy_pass
                          127.0.0.1:8090 — служба garage
                                    │
                          /var/lib/garage/garage.db
```

Сайт доступен только из VPN: nginx слушает адрес VPN, приложение — loopback. Новых портов в файрволе нет.

В `garage.conf` включено сжатие статики (JS, CSS, wasm сканера, SVG, манифест; ответы API не сжимаются), общий предел тела запроса 1 МБ, для `/api/import` — 100 МБ и увеличенные таймауты.

## Что где лежит на сервере

| Что | Путь | Источник в репозитории |
|---|---|---|
| Программа | `/usr/local/bin/garage` | `make build-linux` → `bin/garage-linux-amd64` |
| Настройки | `/etc/garage/config.json` (root:garage, 0640) | `deploy/config.json` |
| Служба | `/etc/systemd/system/garage.service` | `deploy/garage.service` |
| База | `/var/lib/garage/garage.db` (garage, 0700) | создаётся при первом запуске |
| Сайт nginx | `/etc/nginx/conf.d/garage.conf` | `deploy/garage.nginx.conf` |
| Снимки базы | `/var/lib/garage/backup/garage-ГГГГ-ММ-ДД.db` | — |
| Скрипт снимка | `/usr/local/sbin/garage-snapshot` | `deploy/garage-snapshot` |
| Таймер снимка | `garage-snapshot.service`, `garage-snapshot.timer` | `deploy/garage-snapshot.*` |

Служба работает от системного пользователя `garage` без входа в систему, с ограничениями systemd (`ProtectSystem=strict`, `NoNewPrivileges` и др.) и потолком памяти 512 МБ (`MemoryMax`).

## Обновление

С Mac, из корня репозитория:

```bash
make deploy HOST=relay
```

Команда собирает интерфейс и статический бинарник под linux/amd64, копирует его на сервер, заменяет `/usr/local/bin/garage` и перезапускает **только** службу garage. Сессии пользователей хранятся в базе и сохраняются. Миграции базы применяются при запуске сами.

Проверка после обновления:

```bash
curl -s https://garage.north-flow.online/api/health
```

В ответе `version` — коммит, из которого собран бинарник.

Интерфейс на телефоне хранит сервис-воркер (ФТ-12). После выкладки новая версия подхватывается при следующем открытии приложения со связью; старые файлы удаляются сами.

## Первичная установка

Выполнена 18.10.2026. Нужна заново только на чистом сервере. Файлы — из каталога `deploy/`, бинарник — `make build-linux`.

1. Скопировать файлы на сервер, например в `/root/garage-install/`.
2. Пользователь и файлы:

   ```bash
   useradd --system --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin garage
   install -m 0755 garage-linux-amd64 /usr/local/bin/garage
   install -d -m 0750 -o root -g garage /etc/garage
   install -m 0640 -o root -g garage config.json /etc/garage/config.json
   install -m 0644 garage.service garage-snapshot.service garage-snapshot.timer /etc/systemd/system/
   install -m 0755 garage-snapshot /usr/local/sbin/garage-snapshot
   systemctl daemon-reload
   systemctl enable --now garage garage-snapshot.timer
   ```

3. nginx — отдельным файлом, существующие сайты не трогаются. Перезагрузка плавная и только после успешной проверки:

   ```bash
   install -m 0644 garage.nginx.conf /etc/nginx/conf.d/garage.conf
   nginx -t && systemctl reload nginx
   ```

4. AdGuard Home (https://dns.north-flow.online): «Фильтры → Перезапись DNS-запросов» → домен `garage.north-flow.online`, ответ `10.211.7.1`. Добавлять через веб-интерфейс: правка `AdGuardHome.yaml` требует перезапуска AdGuard, а это прерывает DNS для всех клиентов VPN.
5. Ночная копия: в `/usr/local/sbin/node-backup`, в список `PATHS` роли `relay`, добавлены `etc/garage var/lib/garage/backup`.
6. Учётная запись владельца — см. «Учётные записи».

## Telegram-бот

Бот работает внутри службы garage и сам опрашивает Telegram, входящих подключений нет. С relay api.telegram.org напрямую недоступен, поэтому соединения бота привязаны к туннелю `awg1` (`telegram.interface`), как у ночной копии. Прав сверх обычных службе не нужно.

Настройки — блок `telegram` в `/etc/garage/config.json`:

```json
"telegram": {"token": "", "interface": "awg1", "site_url": "https://garage.north-flow.online"}
```

Токен от @BotFather владелец вписывает сам, в журнал он не попадает:

```bash
ssh -t relay "nano /etc/garage/config.json"
ssh relay "systemctl restart garage && sleep 3 && journalctl -u garage -n 5 --no-pager"
```

В журнале должно появиться «telegram: бот подключён». Пустой токен отключает бота, сайт работает как обычно. Если Telegram недоступен, бот повторяет попытки с паузой до минуты.

## Погода

Прогноз и поиск города запрашивает служба garage у Open-Meteo (`api.open-meteo.com`, `geocoding-api.open-meteo.com`). С relay они доступны напрямую, туннель не нужен; ключ не требуется. Прогноз хранится в памяти службы и обновляется не чаще раза в час на каждое место.

Настройки — блок `weather` в `/etc/garage/config.json`:

```json
"weather": {"default": {"name": "Донецк", "lat": 48.023, "lon": 37.802}, "base_url": "", "geocoding_url": "", "timezone": "Europe/Moscow"}
```

- `default` — город по умолчанию для тех, кто не выбрал свой в профиле. Без него и без своего города погода пользователю не показывается.
- `timezone` — часовой пояс часов и дней прогноза. `auto` не подходит: для Донецка Open-Meteo выбирает Europe/Kyiv.
- `base_url` и `geocoding_url` пусты — стандартные адреса (задаются только для тестов).

Если Open-Meteo недоступен, сайт показывает последний полученный прогноз с пометкой времени; остальное работает как обычно.

## Учётные записи

Команды выполняются на сервере от пользователя `garage`. Пароль вводится в терминале дважды и не отображается. Вводите его в английской раскладке.

```bash
ssh -t relay "runuser -u garage -- /usr/local/bin/garage -config /etc/garage/config.json user add <логин>"
ssh -t relay "runuser -u garage -- /usr/local/bin/garage -config /etc/garage/config.json user passwd <логин>"
ssh relay "runuser -u garage -- /usr/local/bin/garage -config /etc/garage/config.json user list"
```

`user add` делает владельцем первого пользователя базы, остальных — по флагу `-owner`. Друзей удобнее приглашать из интерфейса: «Ещё → Профиль → Приглашения» (только у владельца). `user passwd` сбрасывает все сессии пользователя.

## Резервное копирование

- В 22:50 МСК `garage-snapshot.timer` делает снимок базы через `VACUUM INTO`. Это безопасно при работающей службе. Хранятся 7 последних снимков.
- В 23:05 МСК существующая ночная копия relay (`node-backup`) архивирует `/etc/garage` и `/var/lib/garage/backup`, шифрует архив age и отправляет его в Telegram.

Снимок вручную:

```bash
ssh relay "runuser -u garage -- /usr/local/sbin/garage-snapshot && ls -l /var/lib/garage/backup"
```

## Восстановление из копии

Проверено 18.10.2026: снимок с relay поднят на Mac, сервер запустился, учётная запись и справочники на месте.

1. Взять снимок `garage-ГГГГ-ММ-ДД.db`:
   - с сервера — из `/var/lib/garage/backup/`;
   - из Telegram — расшифровать архив закрытым ключом age и взять `var/lib/garage/backup/garage-ГГГГ-ММ-ДД.db`:

     ```bash
     age -d -i <закрытый-ключ> relay-ГГГГ-ММ-ДД.tar.gz.age | tar -xzf - var/lib/garage/backup
     ```

2. Проверить целостность:

   ```bash
   sqlite3 garage-ГГГГ-ММ-ДД.db "PRAGMA integrity_check;"
   ```

   Ответ должен быть `ok`.
3. Вернуть базу на сервер. Служба на это время останавливается:

   ```bash
   scp garage-ГГГГ-ММ-ДД.db relay:/root/garage-restore.db
   ssh relay 'systemctl stop garage \
     && mv /var/lib/garage/garage.db /var/lib/garage/garage.db.broken \
     && rm -f /var/lib/garage/garage.db-wal /var/lib/garage/garage.db-shm \
     && install -m 0600 -o garage -g garage /root/garage-restore.db /var/lib/garage/garage.db \
     && systemctl start garage && systemctl is-active garage'
   ```

4. Проверить вход на сайте. Повреждённую базу `garage.db.broken` удалить, когда всё в порядке.

Проверить снимок, не трогая сервер, можно на Mac: положить его как `garage.db` во временный каталог, указать этот каталог в `data_dir` временного конфига и запустить `./bin/garage -config <конфиг> user list`.

## Диагностика

| Симптом | Что проверить |
|---|---|
| Сайт не открывается | VPN включён? `systemctl is-active garage nginx`; `curl -s http://127.0.0.1:8090/api/health` на сервере |
| Имя не находится | Перезапись в AdGuard; на Mac после её добавления сбросить кэш DNS: `sudo dscacheutil -flushcache; sudo killall -HUP mDNSResponder` |
| «Слишком много попыток» | После 5 неверных паролей вход по логину и адресу закрыт на 15 минут. Счётчик хранится в памяти службы, `systemctl restart garage` сбрасывает его сразу |
| Ошибки службы | `journalctl -u garage -n 50 --no-pager` |
| Бот молчит | `journalctl -u garage -g telegram -n 20 --no-pager`; туннель: `ip -br addr show awg1` |
| Нет погоды | `journalctl -u garage -g погода -n 20 --no-pager`; доступ: `runuser -u garage -- curl -sS -o /dev/null -w '%{http_code}' https://api.open-meteo.com/v1/forecast?latitude=48&longitude=37.8&hourly=temperature_2m` |
| `make deploy` падает на `npm ci` с ENOTEMPTY | `trash web/node_modules` и повторить |
| Запросы к сайту | `/var/log/nginx/access.log` |

Перед обновлением с новыми миграциями полезно снять снимок вручную (см. «Резервное копирование») и скопировать его под другим именем — ротация удаляет только `garage-*.db`.

Известное ограничение: на iOS 27.2 «На экран «Домой»» ставит вместо иконки букву у любого сайта, включая github.com. Это ошибка системы. После обновления iOS ярлык нужно удалить и добавить заново.
