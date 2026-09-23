# opeco.link Privacy Policy

## 1. About this policy

- **Operator:** Satoshi Yazawa (GitHub: [yacchin1205](https://github.com/yacchin1205)).
- **Scope:** The opeco.link relay service, web app, iPhone, iPad, and Mac apps, and the official `opeco` CLI and MCP server.
- **Effective date:** September 23, 2026.

opeco.link temporarily connects a program performing a task with your devices to exchange status updates, notifications, questions, answers, messages, and photos. There are no user accounts, and you do not need to register your name or email address to use the service. On first use, the app automatically registers your device and uses generated identifiers and cryptographic keys to establish connections.

The source code is publicly available. Third parties that operate services using this code or distribute modified software determine their own data handling practices. When third-party software connects to the opeco.link relay service, this policy still applies to data handled by that relay service.

## 2. Information we handle and why

### Communication content

- **Information:** Session titles, status updates, notification text, questions and choices, answers, messages, and attached photos.
- **Purpose:** Delivering this content between connected programs and devices.
- **Handling:** Content is encrypted by the sender and decrypted by the connected recipient. The relay server stores and forwards encrypted content but does not hold the private keys needed to decrypt it.
- **Receiving program:** When you send answers, messages, or photos to the official CLI or MCP server, it decrypts them and passes them to the connected program.
- **Camera and photos:** On iPhone and iPad, the camera is used to scan QR codes and take photos. You can change camera permissions in the operating system settings. Photos you select, take, or share are sent only when you choose to send them.

### Connection and delivery information

We handle the following information to authenticate communications, deliver content, and support sharing across devices. The relay server can observe connection relationships, communication times, and the size of encrypted data.

| Information | Purpose |
| --- | --- |
| Device, group, session, event, and attachment identifiers; delivery destinations | Identifying connections and the information to deliver |
| Public keys, hashes of authentication credentials, signatures, and encrypted shared keys | Authenticating communications and sharing keys among approved devices |
| Group membership and change history, device addition requests and their results, and session participation records | Supporting sharing across devices and tracking participation |
| Creation, update, and expiration times; encrypted data sizes; outstanding items; status update notification preferences | Managing expiration, delivery, and notifications |
| Apple push notification device tokens and notification environment | Delivering notifications to iPhone, iPad, and Mac |

There is no registration system that links these identifiers to names or email addresses. However, the identifiers allow the service to recognize continued use by the same device or group.

### Operational logs and inquiries

- **Operational logs:** We use Cloudflare Workers Logs to record access times, request URLs, processing results, error details, and related request and response metadata to monitor service operation and investigate problems. URLs may contain device or session identifiers. We do not export these logs to other services. Cloudflare also processes source IP addresses and other connection information to deliver and protect the service.
- **Inquiries:** We handle the sender's email address and display name, message text, and attachments included in inquiry emails to investigate and respond to inquiries and follow up on related questions.

The apps do not include advertising or cross-service tracking for advertising purposes. We do not use additional analytics services.

## 3. Information stored on your devices

The apps do not provide us with readable copies of locally stored private keys or decrypted communication content, such as messages and photos. Section 2 describes the encrypted content and connection and delivery information, such as device identifiers and public keys, that the server receives during communication.

### iPhone, iPad, and Mac apps

- The apps store cryptographic keys, connection information, and received content on your device to maintain connections and display received content.
- On Mac, titles, summaries, and other information displayed in the widget are also stored in a local file shared by the app and the widget.

### Web app

- Cryptographic keys, connection information, and received content are stored in your browser.
- Storage is separate for each browser and browser profile.

## 4. Third-party services

### Service providers

| Provider | Purpose and information handled |
| --- | --- |
| Cloudflare | Web hosting, communication relay, data storage, and operational logs. In addition to the server-side information described in Section 2, Cloudflare handles connection information such as source IP addresses. Session content is handled in encrypted form. |
| Apple | Notification delivery through Apple Push Notification service. We send device tokens, generic text indicating that updates or questions are available, outstanding item counts, and related delivery information. Notifications do not include session titles, message content, or photos. |
| Google | Receiving and storing inquiry emails through Google Groups, including the sender's email address and display name, message text, and attachments. |

You can turn off push notifications for iPhone, iPad, and Mac in the operating system's notification settings.

Membership of the Google group used for inquiries is restricted to the maintainers. Inquiry messages are not made public.

### Connected AI agents and third-party services

- Answers, messages, and photos may be passed to AI agents or third-party services through the CLI, MCP server, or other connected programs.
- Their use, retention, and deletion of information they receive depend on the configuration of those programs or services and their providers' policies.

## 5. Retention and deletion

A session expires 24 hours after it is created or last renewed by an event sent by the program that created it. Viewing, syncing, or sending answers from a device does not, by itself, extend this period. The program that created a session can also close the entire session before it expires.

| Information | Storage location | Retention and deletion |
| --- | --- | --- |
| Encrypted communication content and connection and delivery information within a session | Active server storage | Deleted as part of session closure or expiration. |
| Encrypted photo attachments | Active server storage | Deleted after the program that created the session acknowledges receipt. Remaining attachments are deleted as part of session closure or expiration. |
| Earlier versions of database records | Cloudflare database recovery history | Encrypted session content and connection and delivery records may remain recoverable for up to 30 days after deletion from active storage. Photo files are not included. |
| Device registrations, push notification device tokens, groups and their change history, and device addition request records | Server | No fixed automatic deletion period. Closing a session or allowing a device addition request to expire does not, by itself, delete these records. |
| Session participation records held by groups | Server | Deleted during explicit session closure or when group synchronization confirms that participation has ended. Deletion may not coincide exactly with session expiration. |
| Operational logs | Cloudflare Workers Logs | Retained for up to 7 days after each log entry is recorded. |
| Session information in the apps | Device or browser | Deleted when the app detects closure or expiration, such as during startup or synchronization. Information may remain while the app is not running. |
| Cryptographic keys and device and group connection information | Device or browser | Retained for continued use. Session expiration alone does not delete this information. |
| Decrypted photos received by the CLI or MCP server, and session information (including cryptographic keys) saved in shell mode | Temporary storage on the computer running the CLI or MCP server | Deleted when the session is explicitly closed. In shell mode, files are also deleted when the CLI detects that the session has expired or no longer exists. If the program stops without cleanup, files may remain with no fixed automatic deletion deadline. |
| Sender email addresses and display names, message text, and attachments in inquiry emails | Google Groups and the operator's email inbox | Retained to investigate and respond to inquiries and related questions, then deleted from both locations within one year after the inquiry is resolved. |

We do not keep separate backups of relay data.

### Deleting local data and ending device sharing

- **Ending device sharing (all apps):** When multiple devices share a group, you can remove a device from that group through the app. Removal cannot retract information the device has already received and does not delete server-side device registrations or group change history.
- **Deleting browser data (web app):** Clearing the site's data in your browser removes the cryptographic keys, connection information, and received content stored in that browser. It does not delete server-side registrations.

## 6. Contact and changes to this policy

- **Contact:** For questions about this policy or how information is handled, email [opeco-link-support@googlegroups.com](mailto:opeco-link-support@googlegroups.com).
- **When contacting us:** Do not send private keys or unused pairing links or QR codes.
- **Changes:** If our data handling practices change, we will update this page and state the revision date and what changed.
