# Getting started on SERVER_NAME

SERVER_NAME runs wAthena, a Ragnarok Online server that you play in your browser. You need no download and no game client.

> Server owners: replace `SERVER_NAME` with your server's name and `https://play.example.com` with its address. Then link this page or copy it to your site. Fill in the contact line under "If something breaks".

## What you need

- A Chromium-based browser on a computer, such as Google Chrome, Microsoft Edge or Brave. Firefox and Safari are not supported.
- A keyboard and mouse. Touch input works on phones and tablets, but the game is made for a keyboard and mouse.
- The first start downloads the game's art. Later starts load it from your browser's cache.

## 1. Make an account

1. Open `https://play.example.com/`. The game sends you to the login page.
2. Choose "Sign up".
3. Enter an account name and a password, twice.
   - The account name is 4 to 24 letters, digits or underscores, and starts with a letter. Other players never see it. They see your character names.
   - The password needs at least 8 characters, and must not be a common one. Use a password you do not use anywhere else.
4. Press "Sign up".

The server asks for no email address. Keep your password safe: SERVER_NAME cannot send you a reset link.

## 2. Protect your account

Turn on two-factor login, or add a passkey, or both. Both are optional and recommended.

Open "Account settings" in the menu at the top of every account page.

Two-factor login with an app:

1. Press "Turn on two-factor login".
2. Scan the QR code with an authenticator app, such as Aegis, FreeOTP, Google Authenticator or 1Password.
3. Enter the 6-digit code the app shows, and press "Turn on".
4. Save the recovery codes in a safe place. They are shown only once. Each one logs you in once if you lose your phone.

From then on, each login asks for the code from your app after the password.

A passkey logs you in with your phone, computer or security key, without a password:

1. Under "Passkeys", give it a name, such as "Laptop".
2. Press "Add a passkey" and follow your browser's prompt.
3. Next time, press "Log in with a passkey" on the login page.

## 3. Make a character

After you log in, the Characters page opens.

1. Enter a name: 4 to 24 letters or digits, with single spaces between words.
2. Choose a slot, a sex, a hair style and a hair color.
3. Press "Make character".
4. Press "Play" next to the character.

The game opens with your character in the first town. Every character starts as a Novice. The "Characters" button in the game goes back to this page.

## 4. Controls

Mouse:

| Action | What it does |
|---|---|
| Left click on the ground | Walk there. |
| Left click on a monster | Attack it. |
| Left click on an item on the ground | Pick it up. |
| Left click on an NPC | Talk to it. Escape closes the dialog. |
| Right click on a player | Open the player menu: whisper, trade, party and guild invites, and shops. |
| Drag with the left button | Turn the camera. |
| Mouse wheel | Zoom. |

Keyboard:

| Key | What it does |
|---|---|
| Enter | Open the chat box. Enter again sends the line. Escape leaves it. |
| F1 to F9 | Use the skill in that hotkey slot. Drag a skill from the Skills window onto a slot. Right click a slot to clear it. |
| Escape | Cancel a skill that waits for a target, or close a dialog. |
| Alt+A | Status |
| Alt+E | Items. Double click an item, or press Enter, to use or equip it. Delete drops it. |
| Alt+Q | Equipment |
| Alt+S | Skills |
| Alt+W | Cart |
| Alt+Z | Party |
| Alt+B | Party booking |
| Alt+G | Guild |
| Alt+H | Friends |
| Alt+M | Mail |
| Alt+U | Quests |
| Ctrl+` | World map |

Every window also has a button at the top of the screen.

## 5. Chat

Type in the chat box and press Enter. Local chat reaches the players near you.

| Type | To |
|---|---|
| `/w Name hello` | Whisper to a player. Use quotes for names with spaces: `/w "Some Name" hello`. |
| `/p hello` or `%hello` | Your party. |
| `/g hello` or `$hello` | Your guild. |
| `#trade hello` | The `#trade` channel. Other channels: `#global`, `#support` and `#map`. |
| `/join #name`, `/leave #name` | Join or leave a channel. |
| `/channels` | List the channels. |

Each channel, your party, your guild and each whisper has its own tab above the chat box.

## 6. Blocking and reporting

To block a player:

- Type `/block Name`, or open the Blocked panel and enter the name.
- A blocked player cannot whisper to you, trade with you or invite you, and you do not see their chat.
- `/unblock Name` removes the block. Each character has its own block list.

To report a player to the staff:

- To report a chat line, press the "!" button next to it, then "Report?" to confirm.
- To report a player, type `/report Name reason`.
- To report a guild's name or emblem, use the player menu.

Each report includes the chat you saw in the last 5 minutes. Staff read every report. "Blocks, reports and sanctions" on the account page shows your reports and whether staff took action.

## 7. The account page

The menu at the top of the account pages has:

| Page | What it does |
|---|---|
| Characters | Make characters and play. |
| Account settings | Two-factor login, recovery codes, passkeys, your password, and "Log out everywhere else". |
| Blocks, reports and sanctions | Your block lists, the reports you sent, and any sanctions on your account. |
| Your data | Download your data, or delete your account. |
| Guild emblem | Upload your guild's emblem, if you lead a guild. |
| Privacy | What SERVER_NAME keeps about you, for how long, and whom to contact. |

Log out everywhere else:

- "Log out everywhere else" in Account settings ends every other login of your account, and their game connections.
- Use it if you logged in on a shared computer, or think someone knows your password. Then change your password.

Download your data:

- "Download my data" on "Your data" gives you one JSON file with your account, characters, items, and the chat and logs the server still keeps.
- It works once a day, and only while none of your characters is in the game.

Delete your account:

- On "Your data", type your account name and press "Delete my account".
- The deletion happens after 14 days. Until then your characters cannot play.
- To change your mind, press "Cancel the deletion" on the Characters page within the 14 days.
- A guild leader must first make another member the leader.

## If something breaks

- If the game shows "The server is restarting", wait a few minutes and reload the page.
- If the page stays blank, check that you use Chrome, Edge or another Chromium-based browser, then reload.
- If you lose your phone, log in with a recovery code, then turn two-factor login off and on again in Account settings.

For anything else, contact the staff of SERVER_NAME: CONTACT. SERVER_NAME runs its own server. The wAthena software project does not run it, and cannot see or change your account.
