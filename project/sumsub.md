# Sumsub · рабочие заметки

Проверено по документации 25 ноября 2026. Реальный прогон в Sandbox предстоит.

| Вопрос | Что установлено | Источник |
| --- | --- | --- |
| Куда ведёт форма? | Это запрос демо. Обязательны продукт, business email и ожидаемый объём проверок. Форма исключает публичные, личные и ролевые адреса. | [Free demo](https://sumsub.com/free-demo/) |
| Что встраивается в страницу? | WebSDK — браузерный виджет. Есть npm-пакет, CDN и React-обёртка. | [Get started with WebSDK](https://docs.sumsub.com/docs/get-started-with-web-sdk) |
| Кто выдаёт токен? | Наш сервер подписывает запрос в Sumsub с учётными данными Sandbox и получает ограниченный по времени SDK access token для конкретного пользователя и уровня проверки. Браузер передаёт токен виджету. | [Generate access token](https://docs.sumsub.com/reference/generate-access-token) |
| Как тестировать? | В Sandbox доступны тестовые документы и сценарии. Реальное решение о личности там не принимается. | [Test in Sandbox](https://docs.sumsub.com/docs/test-in-sandbox) |
| Откуда взять результат? | В applicantReviewed ответ GREEN означает одобрение. При RED поле reviewRejectType различает повторную подачу (RETRY) и окончательный отказ (FINAL). | [Receive results via API](https://docs.sumsub.com/docs/receive-and-interpret-results-via-api) |

## Что проверить в первом прототипе

- Открывается WebSDK с нашим verification level и токеном тестового пользователя.
- При истечении токена виджет запрашивает новый для того же пользователя.
- Сервер проверяет подпись уведомления и связывает результат с нужным аккаунтом. Повторная доставка даёт тот же результат.
- После обновления страницы UI показывает сохранённый сервером статус. Событие закрытия виджета само по себе не даёт доступа.

Для конкретной задачи сверяй параметр, событие или пример с актуальной страницей документации. После прогона добавь сюда версию SDK, проверенный сценарий и ссылку на реализацию.

## Подготовка и код этапа 5 · 26 ноября 2026

Пользователь подтвердил разрешение использовать корпоративный Sandbox. По переданным
скриншотам выбран `id-and-liveness`: Standard Individual, загрузка ID-документа и
Advanced liveness; presets отсутствуют. Учётные данные хранятся только локально,
их работоспособность запросом к API ещё не подтверждена.

- [SDK token](https://docs.sumsub.com/reference/generate-access-token): POST
  /resources/accessTokens/sdk, JSON userId/levelName/ttlInSecs. Специальные permissions
  для этого метода не требуются. Используется TTL 600 секунд.
- [Authentication](https://docs.sumsub.com/reference/authentication): HMAC-SHA256 от
  timestamp + uppercase method + URI + точные байты тела. Заголовки X-App-Token,
  X-App-Access-Ts, X-App-Access-Sig. Timestamp должен быть близок к времени сервера.
- [WebSDK](https://docs.sumsub.com/docs/get-started-with-web-sdk): официальный CDN,
  .init(token, expirationHandler), callback возвращает Promise нового токена.
  Версия фактически загруженного CDN в браузере пока не проверена.
- [Sandbox](https://docs.sumsub.com/docs/test-in-sandbox): presets управляют результатом,
  а не экранами. Skip/Approve не удаляют liveness из сценария уровня.
- [Тестовые документы](https://docs.sumsub.com/docs/verification-document-templates):
  есть accepted, resubmission и final rejection; сохранять оригинальные файлы/имена.

Реализация: app/internal/sumsub, app/internal/web/verification.go,
app/internal/web/static/verification.js. Автотесты используют имитации, а не
реальный API. Полный прогон и результат провайдера предстоят; webhook пока отсутствует.

## Тестирование результата и виджета · 26 ноября 2026

Подтверждено по [Simulate review response in Sandbox](https://docs.sumsub.com/reference/simulate-review-response-in-sandbox):
POST /resources/applicants/{applicantId}/status/testCompleted задаёт GREEN либо RED
с RETRY/FINAL и запускает настоящий applicantReviewed webhook. Требуются Sandbox
applicant и ключи; для RED нужны rejectLabels. applicantId можно взять в Dashboard.
Это официальный способ проверить доставку результата без реального решения о личности.
Его ещё не вызывали; реальные ключи и доставка не проверены.

Различаем три вида проверки: локальные имитации нашего кода; настоящее уведомление
Sumsub после testCompleted; прохождение экранов WebSDK с документными шаблонами.
Первое не доказывает доставку провайдером, второе не доказывает прохождение liveness.
Официальные [Sandbox presets](https://docs.sumsub.com/docs/test-in-sandbox) и
[шаблоны документов](https://docs.sumsub.com/docs/verification-document-templates)
дополняют друг друга. Имена оригинальных файлов сохранять. Реальные паспорта не нужны;
полностью искусственное прохождение Advanced liveness пока не подтверждено.

[Webhook manager](https://docs.sumsub.com/docs/webhook-manager): HTTPS на порту 443,
без redirect; соединение в пределах 1 секунды, ответ в пределах 10 секунд. Учитывать
холодный старт Fly. Подпись проверяется по исходным байтам через X-Payload-Digest и
X-Payload-Digest-Alg; для нового webhook по умолчанию HMAC_SHA256_HEX. Секрет webhook
задаётся отдельно. События кнопки тестовой отправки имеют testMode; не путать это с
sandboxMode и симуляцией решения настоящего Sandbox applicant.

Рекомендованный порядок и матрица прогонов сохранены в блоке Б
[черновика этапа 6](stages/06-verification-results.md). Разрешение на использование
корпоративного Sandbox подтверждено ранее; изменение уровня/liveness и запуск облака
не выполнялись в рамках подготовки плана.
