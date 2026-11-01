import { definePlugin } from '@shizukubot/sdk';
export default definePlugin({ id: 'counter', name: 'Persistent counter', version: '1.0.0', commands: { count: async (ctx) => { const n = (await ctx.state.get('count') ?? 0) + 1; await ctx.state.set('count', n); await ctx.reply(`${ctx.config.label}: ${n}`); } } });
