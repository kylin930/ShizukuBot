// Public appearance bootstrap is loaded before the first page content is painted.
(() => {
    'use strict';
    const fallback = 'https://teachermate.oss-cn-qingdao.aliyuncs.com/vokMW-1750340487715-nekoaft_N02_a.png';
    let saved = {...window.ShizukuAppearance};
    function apply(value) {
        mdui.setTheme(value.mode || 'auto');
        if (value.primary_color) mdui.setColorScheme(value.primary_color); else mdui.removeColorScheme();
        const image = document.querySelector('.page-background');
        if (image) {
            image.onerror = () => {image.onerror = null; image.src = fallback;};
            image.src = value.background_url || fallback;
        }
        window.dispatchEvent(new Event('appearancechange'));
    }
    function fromConfig(value) {
        return {background_url:value.background.type === 'default' ? fallback : value.background.value,
            primary_color:value.theme.primary_color, mode:value.theme.mode};
    }
    window.AppearanceUI = {apply, fromConfig, fallback, saved:()=>({...saved}),
        commit(value) {saved = fromConfig(value); apply(saved);}, restore:()=>apply(saved), attachBackground:()=>apply(saved)};
    apply(saved);
})();
