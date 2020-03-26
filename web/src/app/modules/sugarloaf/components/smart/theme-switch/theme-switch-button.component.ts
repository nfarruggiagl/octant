// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
//
import { Component, Input, OnInit, Renderer2 } from '@angular/core';
import {
  darkTheme,
  lightTheme,
  Theme,
  ThemeService,
  ThemeType,
} from './theme-switch.service';

@Component({
  selector: 'app-theme-switch-button',
  templateUrl: './theme-switch-button.component.html',
  styleUrls: ['./theme-switch-button.component.scss'],
  providers: [ThemeService],
})
export class ThemeSwitchButtonComponent implements OnInit {
  themeType: ThemeType;
  @Input() public collapsed: boolean;

  constructor(
    private themeService: ThemeService,
    private renderer: Renderer2
  ) {}

  ngOnInit() {
    this.themeType = this.themeService.currentType();
    this.loadTheme();
  }

  isLightThemeEnabled(): boolean {
    // TODO: this should be in the theme service.
    return this.themeType === 'light';
  }

  loadTheme() {
    // TODO: this should be in the theme service.
    const theme: Theme = this.isLightThemeEnabled() ? lightTheme : darkTheme;

    this.themeService.loadCSS(theme.assetPath);

    [darkTheme, lightTheme].forEach(t =>
      this.renderer.removeClass(document.body, t.type)
    );
    this.renderer.addClass(document.body, theme.type);
  }

  switchTheme() {
    // TODO: this should be in the theme service.
    if (this.isLightThemeEnabled()) {
      this.themeType = 'dark';
      localStorage.setItem('theme', 'dark');
    } else {
      this.themeType = 'light';
      localStorage.setItem('theme', 'light');
    }

    this.loadTheme();
  }
}
