import {definePlugin} from '@shizukubot/sdk';
export default definePlugin<{label:string}>({id:'counter',name:'Persistent counter',version:'1.0.0',commands:{count:async(ctx)=>{const n=(await ctx.state.get<number>('count')??0)+1;await ctx.state.set('count',n);await ctx.reply(`${ctx.config.label}: ${n}`);}}});
