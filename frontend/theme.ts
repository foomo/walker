import * as styledComponents from "styled-components";
import { ThemedStyledComponentsModule } from "styled-components";

export interface ThemeInterface {
    primaryColor: string;
    primaryColorInverted: string;
  }

const myStyledComponents = styledComponents as ThemedStyledComponentsModule<ThemeInterface>;

export const styled = myStyledComponents.default;

// export { styled, css, injectGlobal, keyframes, ThemeProvider };
// export styled;