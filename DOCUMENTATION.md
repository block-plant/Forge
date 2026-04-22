# 📖 Forge: The "Simple English" Manual

Welcome to the Forge documentation! If you're reading this, you probably want to know how this massive Backend-as-a-Service (BaaS) actually works under the hood.

Instead of hitting you with a wall of complex Go jargon, we are going to explain the entire Forge architecture using simple, real-world analogies.

---

## 🏢 1. The Multi-Tenant Architecture (The Apartment Building)

Imagine you want to host 10 different apps (a blog, an e-commerce site, and a chat app). Normally, you'd need to set up 10 different servers, 10 different databases, and pay 10 different bills.

Forge uses a **Multi-Tenant Architecture**, which works like an **Apartment Building**:
- **The Landlord (Port 8080):** There is one Master Admin Console running on port 8080. It manages the building. It doesn't hold user data; it just creates the apartments.
- **The Apartments (Port 8081, 8082...):** Every time you create a new "Project", Forge builds a brand-new, locked apartment. Your blog gets port 8081, your e-commerce site gets 8082. They cannot see each other, they don't share data, and if one crashes, the others are totally fine!

---

## 🗄️ 2. The Database (The Filing Cabinet & The Diary)

Forge doesn't use Postgres, MySQL, or MongoDB. We built our own database completely from scratch. 

How does it keep your data safe and fast?
- **The Filing Cabinet (In-Memory B-Tree):** To make searching incredibly fast, Forge keeps all your current documents organized in the computer's active memory (RAM). It's like having a filing cabinet sitting right on your desk.
- **The Diary (Write-Ahead Log - WAL):** But what if the power goes out? Before Forge puts a file in the cabinet, it quickly scribbles a note in its "Diary" (a file saved permanently on the hard drive). If the server crashes, when Forge wakes up, it reads the Diary and puts the filing cabinet back together exactly how it was!

---

## 🔐 3. Authentication (The Bouncer & The VIP Wristband)

When a user signs up for your app, Forge acts like a strict bouncer at a club.

- **The Bouncer (Bcrypt Hashing):** When a user creates a password, Forge scrambles it using complex math (Blowfish cipher) before saving it. Even if a hacker steals the hard drive, the passwords look like random gibberish.
- **The VIP Wristband (JWT Tokens):** Once a user logs in successfully, Forge hands them a digital VIP wristband (a JSON Web Token). Every time their browser asks the server for private data, it flashes the wristband. The server instantly knows who they are without asking for their password again.
- **The Secret Handshake (SMTP OTPs):** If they forget their password, Forge uses a third-party postman (like Brevo) to email them a secret 6-digit code. They must bring that code back to the bouncer to get a new wristband.

---

## 📻 4. Real-Time WebSockets (The Live Walkie-Talkies)

Normally, if you want to know if you got a new message on a website, your browser has to keep asking the server over and over: *"Any new messages? Any new messages?"* This wastes a ton of energy.

Forge solves this using **WebSockets**, which act like a direct **Walkie-Talkie connection**:
1. When your user opens the app, they turn on their Walkie-Talkie and say, *"I am listening to channel: ChatRoom A"*.
2. The server keeps the line open.
3. The moment someone else posts a message in the Database filing cabinet, Forge grabs the Walkie-Talkie and shouts the new message directly to everyone listening. The screen updates instantly—no refreshing required!

---

## 📁 5. File Storage (The Magic Warehouse)

When users upload profile pictures or videos, Forge puts them in the Storage Engine.

- **The Deduplication Magic:** Imagine 1,000 users all upload the exact same funny meme image. A normal server would save 1,000 copies and waste hard drive space. Forge looks at the *actual content* of the image. If it recognizes that it's the exact same picture, it only saves **one copy** in the warehouse, but gives all 1,000 users a map to find it. This saves massive amounts of space!
- **The Temporary Pass (Signed URLs):** If a file is secret (like a private invoice), you can't just send someone a link. Forge lets you create a "Temporary Pass"—a special link that automatically self-destructs after 1 hour.

---

## ⚙️ 6. Serverless Functions (The Hired Robots)

Sometimes you need to run special code, like sending a welcome email or charging a credit card, but you don't want to run it on the user's phone for security reasons.

- You can write a tiny piece of JavaScript and upload it to Forge.
- Forge keeps it safely locked away.
- When you click a button in your app, Forge wakes up a "Hired Robot" (a secure sandbox), hands it your JavaScript, runs the code, returns the answer, and then the robot goes back to sleep. You only use power when the code is actively running!

---

## 🛡️ 7. Security Rules (The Legal Contract)

How do you stop User A from deleting User B's posts?

Forge uses a custom language to write Security Rules. It's like writing a legal contract for your database:
> *"Allow writing to this document ONLY IF the user's ID matches the document's author ID."*

We built a custom engine that reads these English-like rules, translates them into computer logic (using a Lexer and Parser), and enforces them on every single database request. If a hacker tries to bypass the app and talk directly to the database, the rules engine will block them instantly.

---

## 🚫 8. Zero Dependencies (The Hand-Built Engine)

Here is the most impressive part about Forge: **It is built 100% from scratch.**

Most modern software is like a LEGO house built using huge pre-made blocks made by other companies (like PostgreSQL, Redis, or NGINX). 
Forge is different. We didn't use pre-made blocks. We mined the clay and baked the bricks ourselves.

- We didn't use an HTTP library. We wrote code to listen to raw electrical signals (TCP sockets) and parse the text byte-by-byte.
- We didn't use a WebSocket library. We read the official Internet Engineering Task Force rulebook (RFC 6455) and built it ourselves.

Because of this, Forge is incredibly tiny, lighting fast, and we understand exactly how every single byte flows through the system.

---

### You are now ready to use Forge! 
Head over to `INTEGRATION.md` to see exactly how to connect your frontend app to this massive engine.
