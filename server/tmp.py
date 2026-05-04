import imaplib
import email
from email.header import decode_header

# Email credentials
email_user = "whsunqzi@163.com"
password = "AGYJQApRQ8eXkf6b"

# Connect to IMAP server
mail = imaplib.IMAP4_SSL("imap.163.com", 993)

# Add ID command support for Netease
imaplib.Commands["ID"] = ("AUTH",)

# Login first, then send ID command
mail.login(email_user, password)
mail._simple_command("ID", '("name" "UbuntuCLI" "version" "1.0")')

mail.select("INBOX")

# Search for ALL emails to find any from Qizhen
status, messages = mail.search(None, "ALL")
if status != "OK":
    print("Failed to search emails.")
    exit(1)

email_ids = messages[0].split()
if not email_ids:
    print("No emails found in inbox.")
    exit(0)

print(f"Total emails in inbox: {len(email_ids)}")

# Look for emails from Qizhen by scanning recent ones
for e_id in reversed(email_ids[-20:]):  # Check last 20 emails
    status, msg_data = mail.fetch(e_id, "(BODY[HEADER.FIELDS (FROM SUBJECT)])")
    if status != "OK":
        continue
    header_data = msg_data[0][1].decode()
    if "sunqizhen6@gmail.com" in header_data.lower():
        # Found one, fetch full email
        status, msg_data = mail.fetch(e_id, "(RFC822)")
        if status != "OK":
            continue
        msg = email.message_from_bytes(msg_data[0][1])
        
        # Decode subject
        subject, encoding = decode_header(msg["Subject"])[0]
        if isinstance(subject, bytes):
            subject = subject.decode(encoding if encoding else "utf-8")
        
        # Get sender
        from_ = msg.get("From")
        
        # Extract email body
        body = ""
        if msg.is_multipart():
            for part in msg.walk():
                content_type = part.get_content_type()
                content_disposition = str(part.get("Content-Disposition"))
                if content_type == "text/plain" and "attachment" not in content_disposition:
                    body = part.get_payload(decode=True).decode()
                    break
        else:
            body = msg.get_payload(decode=True).decode()
        
        print(f"From: {from_}")
        print(f"Subject: {subject}")
        print("\nContent:")
        print(body)
        break
else:
    print("No emails from Qizhen found in recent emails.")

mail.logout()
