import { createApp } from "vue";
import App from "./App.vue";
import SettingsWindow from "./windows/SettingsWindow.vue";
import DriveWindow from "./windows/DriveWindow.vue";
// vue-sonner 的定位与布局样式，缺少它 toast 不会显示
import "vue-sonner/style.css";
import "./style.css";

// 同一个前端包同时服务主窗口与子窗口，按窗口 URL 的 hash 决定渲染哪个界面
function rootComponent() {
  const hash = window.location.hash;
  if (hash.startsWith("#/settings")) return SettingsWindow;
  if (hash.startsWith("#/drive")) return DriveWindow;
  return App;
}

createApp(rootComponent()).mount("#app");
