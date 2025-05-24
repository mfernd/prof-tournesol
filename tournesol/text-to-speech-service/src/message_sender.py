import discord

class VoiceSender(discord.Client):
    def __init__(self, user_id, voice_file_path, **kwargs):
        super().__init__(**kwargs)
        self.user_id = user_id
        self.voice_file_path = voice_file_path

    async def on_ready(self):
        print(f'Logged in as {self.user}')
        user = await self.fetch_user(self.user_id)
        try:
            await user.send("Here is your generated voice message 🎧", file=discord.File(self.voice_file_path))
            print("✅ Voice message sent!")
        except Exception as e:
            print(f"❌ Failed to send DM: {e}")
        await self.close()

async def send_voice_dm(token, user_id, voice_file_path):
    intents = discord.Intents.default()
    client = VoiceSender(user_id, voice_file_path, intents=intents)
    await client.start(token)