const lang = document.documentElement.lang || "en";

export function t(key) {
    const translations = window.__glanceI18n;

    if (translations && Object.prototype.hasOwnProperty.call(translations, key)) {
        return translations[key];
    }

    return key;
}

export function isZhLanguage() {
    return lang.startsWith("zh");
}

const zhMonthNames = ["一月", "二月", "三月", "四月", "五月", "六月", "七月", "八月", "九月", "十月", "十一月", "十二月"];
const zhWeekDayNames = ["星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"];
const zhWeekDayAbbrs = ["日", "一", "二", "三", "四", "五", "六"];

export const monthNames = isZhLanguage() ? zhMonthNames : ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];
export const weekDayNames = isZhLanguage() ? zhWeekDayNames : ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
export const weekDayAbbrs = isZhLanguage() ? zhWeekDayAbbrs : ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"];

const minuteInSeconds = 60;
const hourInSeconds = minuteInSeconds * 60;
const dayInSeconds = hourInSeconds * 24;
const monthInSeconds = dayInSeconds * 30.4;
const yearInSeconds = dayInSeconds * 365;

export function timestampToRelativeTime(timestamp) {
    let delta = Math.round((Date.now() / 1000) - timestamp);
    let prefix = "";
    let suffix = "";

    if (delta < 0) {
        delta = -delta;
        prefix = isZhLanguage() ? "" : "in ";
        suffix = isZhLanguage() ? "后" : "";
    } else {
        suffix = isZhLanguage() ? "前" : "";
    }

    if (delta < minuteInSeconds) {
        return prefix + "1" + (isZhLanguage() ? "分钟" : "m") + suffix;
    }
    if (delta < hourInSeconds) {
        return prefix + Math.floor(delta / minuteInSeconds) + (isZhLanguage() ? "分钟" : "m") + suffix;
    }
    if (delta < dayInSeconds) {
        return prefix + Math.floor(delta / hourInSeconds) + (isZhLanguage() ? "小时" : "h") + suffix;
    }
    if (delta < monthInSeconds) {
        return prefix + Math.floor(delta / dayInSeconds) + (isZhLanguage() ? "天" : "d") + suffix;
    }
    if (delta < yearInSeconds) {
        return prefix + Math.floor(delta / monthInSeconds) + (isZhLanguage() ? "个月" : "mo") + suffix;
    }

    return prefix + Math.floor(delta / yearInSeconds) + (isZhLanguage() ? "年" : "y") + suffix;
}

export function localizedDate(date) {
    if (isZhLanguage()) {
        return `${date.getFullYear()}年${date.getMonth() + 1}月${date.getDate()}日`;
    }

    return date.getDate() + " " + monthNames[date.getMonth()];
}

export function zoneDiffText(diffInMinutes) {
    if (diffInMinutes == 0) {
        return "";
    }

    const sign = diffInMinutes < 0 ? "-" : "+";
    const direction = isZhLanguage() ? (diffInMinutes < 0 ? "慢" : "快") : (diffInMinutes < 0 ? "behind" : "ahead");

    diffInMinutes = Math.abs(diffInMinutes);

    const hours = Math.floor(diffInMinutes / 60);
    const minutes = diffInMinutes % 60;

    if (minutes == 0) {
        const hourSuffix = hours == 1 ? "" : "s";
        return { text: `${sign}${hours}h`, title: isZhLanguage() ? `${hours} 小时${direction}` : `${hours} hour${hourSuffix} ${direction}` };
    }

    if (hours == 0) {
        return { text: `${sign}${minutes}m`, title: isZhLanguage() ? `${minutes} 分钟${direction}` : `${minutes} minutes ${direction}` };
    }

    return { text: `${sign}${hours}h~`, title: isZhLanguage() ? `${hours} 小时 ${minutes} 分钟${direction}` : `${hours} hour${hours == 1 ? "" : "s"} and ${minutes} minutes ${direction}` };
}
