// Copyright (c) 2019 the Octant contributors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
//
import { Component, OnDestroy, OnInit } from '@angular/core';
import { BehaviorSubject } from 'rxjs';
import { Navigation, NavigationChild } from '../../../models/navigation';
import { IconService } from '../../../../shared/services/icon/icon.service';
import { NavigationService } from '../../../../shared/services/navigation/navigation.service';
import { untilDestroyed } from 'ngx-take-until-destroy';
import { take } from 'rxjs/operators';

const emptyNavigation: Navigation = {
  sections: [],
  defaultPath: '',
};

@Component({
  selector: 'app-navigation',
  templateUrl: './navigation.component.html',
  styleUrls: ['./navigation.component.scss'],
})
export class NavigationComponent implements OnInit, OnDestroy {
  collapsed = false;
  navExpandedState: any;
  lastSelection: number;
  flyoutIndex = -1;

  navigation = emptyNavigation;

  constructor(
    private iconService: IconService,
    private navigationService: NavigationService
  ) {}

  ngOnInit() {
    this.navigationService.current
      .pipe(untilDestroyed(this))
      .subscribe(navigation => (this.navigation = navigation));
    this.navigationService.expandedState
      .pipe(untilDestroyed(this))
      .subscribe(expanded => (this.navExpandedState = expanded));
    this.navigationService.collapsed
      .pipe(untilDestroyed(this))
      .pipe(take(1))
      .subscribe(collapsed => (this.collapsed = collapsed));
    this.navigationService.lastSelection
      .pipe(untilDestroyed(this))
      .subscribe(selection => (this.lastSelection = selection));
  }

  ngOnDestroy() {}

  identifyNavigationItem(index: number, item: NavigationChild): string {
    return item.title;
  }

  itemIcon(item: NavigationChild): string {
    return this.iconService.load(item);
  }

  formatPath(path: string): string {
    if (!path.startsWith('/')) {
      return '/' + path;
    }

    return path;
  }

  openPopup(index: number) {
    this.clearExpandedState();
    this.setNavState(true, index);
    this.setLastSelection(index);
  }

  closePopups(index) {
    this.clearExpandedState();
    this.flyoutIndex = -1;
    this.setLastSelection(index);
  }

  setLastSelection(index) {
    this.lastSelection = index;
    this.navigationService.lastSelection.next(index);
  }

  setExpandedState(index, state) {
    this.navExpandedState[index] = state;
    this.navigationService.expandedState.next(this.navExpandedState);
  }

  clearExpandedState() {
    this.navExpandedState= {};
    this.navigationService.expandedState.next(this.navExpandedState);
  }

  setNavState($event, state: number) {
    if(this.collapsed) {
      this.setLastSelection(state);
    } else {
      this.setExpandedState(state, $event);
      if ($event && this.lastSelection !== state) {
        // collapse previously selected group
        if (this.lastSelection) {
          this.setExpandedState(this.lastSelection, false);
        }
        this.setLastSelection(state);
      }
    }
  }

  shouldExpand(index: number) {
    if(this.collapsed) {
      return (index == this.flyoutIndex)
    } else if (index.toString() in this.navExpandedState) {
        return this.navExpandedState[index];
    }
    return false;
  }

  updateNavCollapsed(value: boolean): void {
    this.collapsed = value;
    this.navigationService.collapsed.next(value);
    this.setExpandedState(this.lastSelection, false);
  }
}
