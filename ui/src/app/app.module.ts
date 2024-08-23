import { NgModule } from '@angular/core';
import { BrowserModule } from '@angular/platform-browser';

import { AppRoutingModule } from './app-routing.module';
import { AppComponent } from './app.component';
import { Toolbar } from './toolbar/toolbar.component';
import { SearchComponent } from './search/search.component';
import { ScheduleComponent } from './schedule/schedule.component';
import { DashboardComponent } from './dashboard/dashboard.component';

@NgModule({
    declarations: [
        AppComponent
    ],
    imports: [
        BrowserModule,
        AppRoutingModule,
        SearchComponent,
        ScheduleComponent,
        DashboardComponent,
        Toolbar
    ],
    providers: [],
    bootstrap: [AppComponent]
})
export class AppModule { }