import { definePlugin } from '@shizukubot/sdk';
async function respond(ctx) { if (ctx.message.includes(ctx.config.keyword))
    await ctx.reply(ctx.config.reply); }
export default definePlugin({ id: 'keywords', name: 'Keyword replies', version: '1.0.0', events: { group_at_message: respond, group_message: respond, c2c_message: respond } });
