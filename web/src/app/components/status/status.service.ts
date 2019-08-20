import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Tab } from '../../models/status';
import getAPIBase from '../../services/common/getAPIBase';

@Injectable()
export class StatusService {
  constructor(private http: HttpClient) {}

  public getTabs() {
    const url = [getAPIBase(), 'api/v1/octant-status/tabs'].join('/');
    return this.http.get<Tab[]>(url);
  }
}
