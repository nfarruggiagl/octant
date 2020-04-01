// Copyright (c) 2019 the Octant contributors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
//

import { Component, OnDestroy, OnInit } from '@angular/core';
import { untilDestroyed } from 'ngx-take-until-destroy';
import { NamespaceService } from 'src/app/modules/shared/services/namespace/namespace.service';
import trackByIdentity from 'src/app/util/trackBy/trackByIdentity';
import { NavigationService } from '../../../../shared/services/navigation/navigation.service';

@Component({
  selector: 'app-namespace',
  templateUrl: './namespace.component.html',
  styleUrls: ['./namespace.component.scss'],
})
export class NamespaceComponent implements OnInit, OnDestroy {
  namespaces: string[];
  currentNamespace = '';
  trackByIdentity = trackByIdentity;
  navigation = {
    sections: [],
    defaultPath: '',
  };
  lastSelection: number;

  constructor(
    private namespaceService: NamespaceService,
    private navigationService: NavigationService
  ) {}

  ngOnInit() {
    this.namespaceService.activeNamespace
      .pipe(untilDestroyed(this))
      .subscribe((namespace: string) => {
        this.currentNamespace = namespace;
      });

    this.namespaceService.availableNamespaces
      .pipe(untilDestroyed(this))
      .subscribe((namespaces: string[]) => {
        this.namespaces = namespaces;
      });

    this.navigationService.current
      .pipe(untilDestroyed(this))
      .subscribe(navigation => (this.navigation = navigation));

    this.navigationService.lastSelection
      .pipe(untilDestroyed(this))
      .subscribe(selection => (this.lastSelection = selection));
  }

  ngOnDestroy() {}

  namespaceClass(namespace: string) {
    const active = this.currentNamespace === namespace ? ['active'] : [];
    return ['context-button', ...active];
  }

  selectNamespace(namespace: string) {
    this.namespaceService.setNamespace(namespace);
  }

  showDropdown() {
    if (this.lastSelection && this.navigation.sections[this.lastSelection]) {
      return !this.navigation.sections[this.lastSelection].path.includes(
        'cluster-overview'
      );
    }
    return true;
  }
}
