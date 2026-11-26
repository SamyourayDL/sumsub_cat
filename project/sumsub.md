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
