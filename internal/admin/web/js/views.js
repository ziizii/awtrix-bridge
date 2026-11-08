// The pages of the admin UI and the sections on each, in order. Each
// view gets a tab (bottom bar on phones, sidebar on wide screens) and the
// URL #/<id>.
//
// Adding a feature: write a section module in sections/ and list it in a
// view here, or add a view with a new icon from icons.js. A section
// exports:
//
//   mount(ui)       -> Element  built once; the section keeps references
//   render(status, ui)          optional, called with every status poll
//
// A section belongs to one view (modules keep their elements in module
// state). See main.js for what ui offers.

import * as hero from "./sections/hero.js";
import * as pickup from "./sections/pickup.js";
import * as apps from "./sections/apps.js";
import * as chime from "./sections/chime.js";
import * as recent from "./sections/recent.js";
import * as system from "./sections/system.js";
import * as editor from "./sections/editor.js";

export const views = [
  { id: "now", title: "Now", icon: "clock", sections: [hero, pickup, apps] },
  { id: "sounds", title: "Sounds", icon: "bell", sections: [chime] },
  { id: "activity", title: "Activity", icon: "activity", sections: [recent, system] },
  { id: "config", title: "Config", icon: "code", sections: [editor] },
];
