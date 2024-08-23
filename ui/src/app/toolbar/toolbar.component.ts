import { Component, OnInit } from "@angular/core";
import { MenuItem } from 'primeng/api';
import { TabMenuModule } from 'primeng/tabmenu';
import { ToolbarModule } from 'primeng/toolbar';

@Component({
    selector: 'toolbar',
    templateUrl: './toolbar.component.html',
    styleUrls: ['./toolbar.component.scss'],
    standalone: true,
    imports: [TabMenuModule, ToolbarModule]
})

export class Toolbar implements OnInit {
    items: MenuItem[] | undefined;

    ngOnInit() {
        this.items = [
            { label: 'Dashboard', icon: 'pi pi-home', routerLink: '/' },
            { label: 'Schedule', icon: 'pi pi-calendar', routerLink: '/schedule' },
            { label: 'Search', icon: 'pi pi-search-plus', routerLink: '/search' },
        ]
    }
}