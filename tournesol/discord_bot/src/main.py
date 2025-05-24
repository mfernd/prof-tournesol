import discord
import aiohttp
import os
from dotenv import load_dotenv

load_dotenv()

TOKEN = os.getenv("TOKEN")

# Enable intents to receive message events including DMs
intents = discord.Intents.default()
intents.messages = True
intents.dm_messages = True

bot = discord.Client(intents=intents)

WEBHOOK_URL = "http://localhost:8000/webhook"

@bot.event
async def on_ready():
    print(f"Bot is ready. Logged in as {bot.user}")

@bot.event
async def on_message(message):
    if message.author.bot:
        return

    if message.attachments:
        for attachment in message.attachments:
            if any(attachment.filename.lower().endswith(ext) for ext in ['.mp3', '.wav', '.ogg', '.m4a']):
                # Get file bytes
                audio_bytes = await attachment.read()

                # Send file to webhook
                async with aiohttp.ClientSession() as session:
                    data = aiohttp.FormData()
                    data.add_field('file', audio_bytes,
                                   filename=attachment.filename,
                                   content_type=attachment.content_type or 'application/octet-stream')

                    async with session.post(WEBHOOK_URL, data=data) as resp:
                        if resp.status == 200:
                            print(f"File sent to webhook successfully: {attachment.filename}")
                        else:
                            print(f"Failed to send file to webhook: {resp.status}")

# Run the bot with your bot token
bot.run(TOKEN)
